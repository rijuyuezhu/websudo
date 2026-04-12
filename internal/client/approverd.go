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

type Result struct {
	ExitCode int    `json:"exitCode"`
	Signal   int    `json:"signal,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type Request struct {
	ID          string          `json:"id"`
	CreatedAt   time.Time       `json:"createdAt"`
	RequestedBy model.Requester `json:"requestedBy"`
	Command     model.Command   `json:"command"`
	Status      model.Status    `json:"status"`
	Result      *Result         `json:"result,omitempty"`
}

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

func (c *Approverd) CreateAndWait(ctx context.Context, req model.Request) (Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Request{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/requests", bytes.NewReader(body))
	if err != nil {
		return Request{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Request{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return Request{}, fmt.Errorf("create request failed: %s", resp.Status)
	}

	var created Request
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return Request{}, err
	}

	for {
		current, err := c.Get(ctx, created.ID)
		if err != nil {
			return Request{}, err
		}
		switch current.Status {
		case model.StatusSucceeded, model.StatusFailed, model.StatusDenied, model.StatusExpired:
			return current, nil
		}

		select {
		case <-ctx.Done():
			return Request{}, ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Approverd) Get(ctx context.Context, id string) (Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/requests/"+id, nil)
	if err != nil {
		return Request{}, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Request{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Request{}, fmt.Errorf("get request failed: %s", resp.Status)
	}
	var req Request
	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		return Request{}, err
	}
	return req, nil
}
