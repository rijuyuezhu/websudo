package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func (s *SQLiteStore) ListRequestsByStatus(ctx context.Context, status model.Status) ([]model.Request, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, created_at, requester_json, command_json
		FROM requests
		WHERE status = ?
		ORDER BY created_at DESC
	`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRequests(rows)
}

func (s *SQLiteStore) ListRequestsExcludingStatus(ctx context.Context, status model.Status, limit int) ([]model.Request, error) {
	query := `
		SELECT id, status, created_at, requester_json, command_json
		FROM requests
		WHERE status <> ?
		ORDER BY created_at DESC
	`
	args := []any{string(status)}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRequests(rows)
}

func (s *SQLiteStore) UpdateRequestStatus(ctx context.Context, id string, from, to model.Status) (model.Request, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Request{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE requests
		SET status = ?
		WHERE id = ? AND status = ?
	`, string(to), id, string(from))
	if err != nil {
		return model.Request{}, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return model.Request{}, err
	}
	if affected != 1 {
		return model.Request{}, errors.New("request was not pending")
	}

	req, err := getRequest(tx, ctx, id)
	if err != nil {
		return model.Request{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Request{}, err
	}
	return req, nil
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

type rowScanner interface {
	Scan(dest ...any) error
}

type requestQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getRequest(db requestQuerier, ctx context.Context, id string) (model.Request, error) {
	var (
		statusText    string
		createdAtText string
		requesterJSON string
		commandJSON   string
	)

	err := db.QueryRowContext(ctx, `
		SELECT status, created_at, requester_json, command_json
		FROM requests
		WHERE id = ?
	`, id).Scan(&statusText, &createdAtText, &requesterJSON, &commandJSON)
	if err != nil {
		return model.Request{}, err
	}

	return decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON)
}

func scanRequests(rows *sql.Rows) ([]model.Request, error) {
	var requests []model.Request
	for rows.Next() {
		var (
			id            string
			statusText    string
			createdAtText string
			requesterJSON string
			commandJSON   string
		)
		if err := rows.Scan(&id, &statusText, &createdAtText, &requesterJSON, &commandJSON); err != nil {
			return nil, err
		}
		req, err := decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return requests, nil
}

func decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON string) (model.Request, error) {
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
