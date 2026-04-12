package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"websudo/internal/model"
)

type Approverd struct {
	baseURL      string
	httpClient   *http.Client
	pollInterval time.Duration
}

func New(baseURL string, httpClient *http.Client) *Approverd {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Approverd{
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   httpClient,
		pollInterval: 250 * time.Millisecond,
	}
}

func (c *Approverd) CreateAndWait(ctx context.Context, req model.Request) (model.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return model.Request{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/requests", bytes.NewReader(body))
	if err != nil {
		return model.Request{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return model.Request{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return model.Request{}, fmt.Errorf("create request failed: %s", resp.Status)
	}

	var created model.Request
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return model.Request{}, err
	}

	for {
		current, err := c.Get(ctx, created.ID())
		if err != nil {
			return model.Request{}, err
		}
		switch current.Status() {
		case model.StatusSucceeded, model.StatusFailed, model.StatusDenied, model.StatusExpired:
			return current, nil
		}

		select {
		case <-ctx.Done():
			return model.Request{}, ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Approverd) Get(ctx context.Context, id string) (model.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/requests/"+id, nil)
	if err != nil {
		return model.Request{}, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return model.Request{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.Request{}, fmt.Errorf("get request failed: %s", resp.Status)
	}
	var req model.Request
	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		return model.Request{}, err
	}
	return req, nil
}
