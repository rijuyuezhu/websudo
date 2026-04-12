package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"websudo/internal/model"
)

func TestCreateAndWaitPostsRequestThenPollsUntilFinished(t *testing.T) {
	t.Helper()

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /api/requests":
			_, _ = w.Write([]byte(`{"id":"req-1","status":"pending","createdAt":"2026-04-12T06:00:00Z","requestedBy":{},"command":{"resolvedPath":"/usr/bin/true","argv":["/usr/bin/true"],"cwd":"/tmp"}}`))
		case http.MethodGet + " /api/requests/req-1":
			callCount++
			if callCount == 1 {
				_, _ = w.Write([]byte(`{"id":"req-1","status":"running","createdAt":"2026-04-12T06:00:00Z","requestedBy":{},"command":{"resolvedPath":"/usr/bin/true","argv":["/usr/bin/true"],"cwd":"/tmp"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"req-1","status":"succeeded","createdAt":"2026-04-12T06:00:00Z","requestedBy":{},"command":{"resolvedPath":"/usr/bin/true","argv":["/usr/bin/true"],"cwd":"/tmp"},"result":{"exitCode":0,"stdout":"ok"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, ts.Client())
	c.pollInterval = time.Millisecond

	finalReq, err := c.CreateAndWait(context.Background(), model.NewRequest(
		"local-id",
		time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC),
		model.Requester{},
		model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
	))
	if err != nil {
		t.Fatalf("CreateAndWait() error = %v", err)
	}
	if finalReq.Status() != model.StatusSucceeded {
		t.Fatalf("status = %q, want %q", finalReq.Status(), model.StatusSucceeded)
	}
	if finalReq.Result() == nil || finalReq.Result().Stdout != "ok" {
		t.Fatalf("result = %#v, want buffered stdout", finalReq.Result())
	}
	if callCount < 2 {
		t.Fatalf("poll count = %d, want at least 2", callCount)
	}
	if finalReq.ID() != "req-1" {
		t.Fatalf("id = %q, want %q", finalReq.ID(), "req-1")
	}
	if got := finalReq.Command().ResolvedPath; got != "/usr/bin/true" {
		t.Fatalf("resolved path = %q, want %q", got, "/usr/bin/true")
	}
	if createdAt := finalReq.CreatedAt(); !createdAt.Equal(time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("createdAt = %v, want fixed timestamp", createdAt)
	}
}
