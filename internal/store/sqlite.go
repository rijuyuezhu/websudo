package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"websudo/internal/model"
)

type SQLiteStore struct {
	db *sql.DB
}

func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) CreateRequest(ctx context.Context, req model.Request) error {
	requesterJSON, err := json.Marshal(req.RequestedBy())
	if err != nil {
		return err
	}
	commandJSON, err := json.Marshal(req.Command())
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO requests (id, status, created_at, requester_json, command_json)
		VALUES (?, ?, ?, ?, ?)
	`, req.ID(), string(req.Status()), req.CreatedAt().UTC().Format(time.RFC3339Nano), string(requesterJSON), string(commandJSON))
	return err
}

func (s *SQLiteStore) GetRequest(ctx context.Context, id string) (model.Request, error) {
	var (
		statusText    string
		createdAtText string
		requesterJSON string
		commandJSON   string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT status, created_at, requester_json, command_json
		FROM requests
		WHERE id = ?
	`, id).Scan(&statusText, &createdAtText, &requesterJSON, &commandJSON)
	if err != nil {
		return model.Request{}, err
	}

	status, err := parseStoredStatus(statusText)
	if err != nil {
		return model.Request{}, err
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtText)
	if err != nil {
		return model.Request{}, err
	}

	var requester model.Requester
	if err := json.Unmarshal([]byte(requesterJSON), &requester); err != nil {
		return model.Request{}, err
	}

	var command model.Command
	if err := json.Unmarshal([]byte(commandJSON), &command); err != nil {
		return model.Request{}, err
	}

	return model.NewStoredRequest(id, createdAt, requester, command, status), nil
}

func (s *SQLiteStore) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS requests (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			requester_json TEXT NOT NULL,
			command_json TEXT NOT NULL
		)
	`)
	return err
}

func parseStoredStatus(text string) (model.Status, error) {
	status := model.Status(text)
	switch status {
	case model.StatusPending, model.StatusApproved, model.StatusRunning, model.StatusSucceeded, model.StatusFailed, model.StatusDenied, model.StatusExpired:
		return status, nil
	default:
		return "", fmt.Errorf("invalid stored status %q", text)
	}
}
