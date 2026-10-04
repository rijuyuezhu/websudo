package askpass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

type Request struct {
	ID      string
	conn    net.Conn
	decoder *json.Decoder
}

type IPCRequest struct {
	Prompt string `json:"prompt"`
}

type IPCCreated struct {
	ID string `json:"id"`
}

type IPCResult struct {
	Password string `json:"password,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Client struct {
	socketPath string
}

func New(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func (c *Client) Create(ctx context.Context, prompt string) (Request, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return Request{}, fmt.Errorf("connect to websudo approval daemon: %w", err)
	}
	closeConn := true
	defer func() {
		if closeConn {
			_ = conn.Close()
		}
	}()

	stop := closeOnContext(ctx, conn)
	defer func() { _ = stop() }()

	if err := json.NewEncoder(conn).Encode(IPCRequest{Prompt: prompt}); err != nil {
		return Request{}, fmt.Errorf("send askpass request: %w", err)
	}

	decoder := json.NewDecoder(conn)
	var created IPCCreated
	if err := decoder.Decode(&created); err != nil {
		if ctx.Err() != nil {
			return Request{}, ctx.Err()
		}
		return Request{}, fmt.Errorf("receive askpass request id: %w", err)
	}
	if created.ID == "" {
		return Request{}, errors.New("askpass request missing id")
	}
	if !stop() {
		if err := ctx.Err(); err != nil {
			return Request{}, err
		}
	}

	closeConn = false
	return Request{ID: created.ID, conn: conn, decoder: decoder}, nil
}

func (c *Client) WaitForPassword(ctx context.Context, req Request) (string, error) {
	if req.ID == "" {
		return "", errors.New("askpass request missing id")
	}
	if req.conn == nil || req.decoder == nil {
		return "", errors.New("askpass request has no active connection")
	}
	defer func() { _ = req.conn.Close() }()

	stop := closeOnContext(ctx, req.conn)
	defer func() { _ = stop() }()

	var result IPCResult
	if err := req.decoder.Decode(&result); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("receive askpass result: %w", err)
	}
	if result.Error != "" {
		return "", errors.New(result.Error)
	}
	return result.Password, nil
}

func closeOnContext(ctx context.Context, conn net.Conn) func() bool {
	if ctx.Done() == nil {
		return func() bool { return true }
	}
	return context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
}
