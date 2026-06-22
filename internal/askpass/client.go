package askpass

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const consumeTokenHeader = "X-Websudo-Askpass-Token"

var errPending = errors.New("askpass request pending")

type Request struct {
	ID           string    `json:"id"`
	Prompt       string    `json:"prompt"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	ConsumeToken string    `json:"consumeToken"`
}

type Client struct {
	baseURL      string
	httpClient   *http.Client
	pollInterval time.Duration
}

func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   httpClient,
		pollInterval: 250 * time.Millisecond,
	}
}

func (c *Client) Create(ctx context.Context, prompt string) (Request, error) {
	body, err := json.Marshal(struct {
		Prompt string `json:"prompt"`
	}{Prompt: prompt})
	if err != nil {
		return Request{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/askpass", bytes.NewReader(body))
	if err != nil {
		return Request{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Request{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return Request{}, fmt.Errorf("create askpass request failed: %s", resp.Status)
	}

	var req Request
	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		return Request{}, err
	}
	if req.ID == "" {
		return Request{}, errors.New("create askpass request missing id")
	}
	if req.ConsumeToken == "" {
		return Request{}, errors.New("create askpass request missing consume token")
	}
	return req, nil
}

func (c *Client) WaitForPassword(ctx context.Context, req Request) (string, error) {
	if req.ID == "" {
		return "", errors.New("askpass request missing id")
	}
	if req.ConsumeToken == "" {
		return "", errors.New("askpass request missing consume token")
	}

	for {
		password, err := c.consume(ctx, req)
		if err == nil {
			return password, nil
		}
		if !errors.Is(err, errPending) {
			return "", err
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Client) consume(ctx context.Context, req Request) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/askpass/"+url.PathEscape(req.ID)+"/consume", nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set(consumeTokenHeader, req.ConsumeToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return "", err
		}
		return body.Password, nil
	case http.StatusConflict:
		return "", errPending
	case http.StatusGone, http.StatusForbidden, http.StatusNotFound:
		return "", fmt.Errorf("consume askpass request failed: %s", resp.Status)
	default:
		return "", fmt.Errorf("consume askpass request failed: %s", resp.Status)
	}
}
