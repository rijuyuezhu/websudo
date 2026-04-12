package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	resultJSON, err := json.Marshal(req.Result())
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO requests (id, status, created_at, requester_json, command_json, result_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, req.ID(), string(req.Status()), req.CreatedAt().UTC().Format(time.RFC3339Nano), string(requesterJSON), string(commandJSON), string(resultJSON))
	return err
}

func (s *SQLiteStore) GetRequest(ctx context.Context, id string) (model.Request, error) {
	var (
		statusText    string
		createdAtText string
		requesterJSON string
		commandJSON   string
		resultJSON    string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT status, created_at, requester_json, command_json, COALESCE(result_json, 'null')
		FROM requests
		WHERE id = ?
	`, id).Scan(&statusText, &createdAtText, &requesterJSON, &commandJSON, &resultJSON)
	if err != nil {
		return model.Request{}, err
	}

	return decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON, resultJSON)
}

func (s *SQLiteStore) ListRequestsByStatus(ctx context.Context, status model.Status) ([]model.Request, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, created_at, requester_json, command_json, COALESCE(result_json, 'null')
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
		SELECT id, status, created_at, requester_json, command_json, COALESCE(result_json, 'null')
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

func (s *SQLiteStore) CompleteRequest(ctx context.Context, id string, result model.Result) (model.Request, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Request{}, err
	}
	defer tx.Rollback()

	current, err := getRequest(tx, ctx, id)
	if err != nil {
		return model.Request{}, err
	}
	completed, err := current.WithResult(result)
	if err != nil {
		return model.Request{}, err
	}
	resultJSON, err := json.Marshal(completed.Result())
	if err != nil {
		return model.Request{}, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE requests
		SET status = ?, result_json = ?
		WHERE id = ?
	`, string(completed.Status()), string(resultJSON), id)
	if err != nil {
		return model.Request{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Request{}, err
	}
	return completed, nil
}

func (s *SQLiteStore) ExpirePendingRequests(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE requests
		SET status = ?
		WHERE status = ? AND created_at <= ?
	`, string(model.StatusExpired), string(model.StatusPending), before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
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
			command_json TEXT NOT NULL,
			result_json TEXT
		)
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE requests ADD COLUMN result_json TEXT`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
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
		resultJSON    string
	)

	err := db.QueryRowContext(ctx, `
		SELECT status, created_at, requester_json, command_json, COALESCE(result_json, 'null')
		FROM requests
		WHERE id = ?
	`, id).Scan(&statusText, &createdAtText, &requesterJSON, &commandJSON, &resultJSON)
	if err != nil {
		return model.Request{}, err
	}

	return decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON, resultJSON)
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
			resultJSON    string
		)
		if err := rows.Scan(&id, &statusText, &createdAtText, &requesterJSON, &commandJSON, &resultJSON); err != nil {
			return nil, err
		}
		req, err := decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON, resultJSON)
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

func decodeRequestRow(id, statusText, createdAtText, requesterJSON, commandJSON, resultJSON string) (model.Request, error) {
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

	var result *model.Result
	if strings.TrimSpace(resultJSON) != "" && strings.TrimSpace(resultJSON) != "null" {
		var decoded model.Result
		if err := json.Unmarshal([]byte(resultJSON), &decoded); err != nil {
			return model.Request{}, err
		}
		result = &decoded
	}

	return model.NewStoredRequest(id, createdAt, requester, command, status, result), nil
}
