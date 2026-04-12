package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"websudo/internal/model"
)

func TestSQLiteStorePersistsAndLoadsPendingRequest(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()

	req := model.NewRequest(
		"req-sqlite-1",
		time.Date(2026, 4, 12, 4, 0, 0, 0, time.UTC),
		model.Requester{},
		model.Command{
			ResolvedPath: "/usr/bin/true",
			Argv:         []string{"/usr/bin/true"},
			Cwd:          "/tmp",
		},
	)

	if err := s.CreateRequest(context.Background(), req); err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}

	got, err := s.GetRequest(context.Background(), req.ID())
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if got.Status() != model.StatusPending {
		t.Fatalf("expected pending, got %q", got.Status())
	}
	if got.Command().ResolvedPath != "/usr/bin/true" {
		t.Fatalf("unexpected path: %q", got.Command().ResolvedPath)
	}
}

func TestSQLiteStorePreservesStoredStatus(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()

	req := model.NewRequest(
		"req-sqlite-2",
		time.Date(2026, 4, 12, 4, 5, 0, 0, time.UTC),
		model.Requester{},
		model.Command{
			ResolvedPath: "/usr/bin/true",
			Argv:         []string{"/usr/bin/true"},
			Cwd:          "/tmp",
		},
	)

	approved, err := req.Transition(model.StatusApproved)
	if err != nil {
		t.Fatalf("Transition(StatusApproved) error = %v", err)
	}

	if err := s.CreateRequest(context.Background(), approved); err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}

	got, err := s.GetRequest(context.Background(), approved.ID())
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if got.Status() != model.StatusApproved {
		t.Fatalf("expected approved, got %q", got.Status())
	}
}

func TestSQLiteStoreRejectsInvalidStoredStatus(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()

	requesterJSON := `{}`
	commandJSON := `{"ResolvedPath":"/usr/bin/true","Argv":["/usr/bin/true"],"Cwd":"/tmp"}`
	_, err := s.db.Exec(`
		INSERT INTO requests (id, status, created_at, requester_json, command_json)
		VALUES (?, ?, ?, ?, ?)
	`, "req-invalid-status", "bogus", time.Date(2026, 4, 12, 4, 10, 0, 0, time.UTC).Format(time.RFC3339Nano), requesterJSON, commandJSON)
	if err != nil {
		t.Fatalf("insert invalid row error = %v", err)
	}

	_, err = s.GetRequest(context.Background(), "req-invalid-status")
	if err == nil {
		t.Fatal("GetRequest() error = nil, want invalid status error")
	}
	if !strings.Contains(err.Error(), `invalid stored status "bogus"`) {
		t.Fatalf("GetRequest() error = %v, want invalid stored status", err)
	}
}

func newTestStore(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "websudo-test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	return s, func() {
		if err := s.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}
