package approverd

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"websudo/internal/config"
	"websudo/internal/model"
)

func TestSQLiteStoreAdapterImplementsApproverdStore(t *testing.T) {
	var impl any = (*SQLiteStore)(nil)
	if _, ok := impl.(Store); !ok {
		t.Fatal("SQLiteStore adapter must implement approverd.Store")
	}
}

func TestApproveHandlerRequiresValidToken(t *testing.T) {
	srv := NewServer(Dependencies{
		Config: config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store: newMemoryStore([]model.Request{
			model.NewRequest(
				"req-1",
				time.Date(2026, 4, 12, 5, 0, 0, 0, time.UTC),
				model.Requester{Username: "rijuyuezhu"},
				model.Command{ResolvedPath: "/usr/bin/pacman", Argv: []string{"/usr/bin/pacman", "-Syu"}, Cwd: "/tmp"},
			),
		}),
		Templates: testTemplates(t),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-1/approve", strings.NewReader(`{"token":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestDenyHandlerRequiresValidToken(t *testing.T) {
	store := newMemoryStore([]model.Request{
		model.NewRequest(
			"req-2",
			time.Date(2026, 4, 12, 5, 5, 0, 0, time.UTC),
			model.Requester{Username: "rijuyuezhu"},
			model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		),
	})
	srv := NewServer(Dependencies{
		Config:    config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:     store,
		Templates: testTemplates(t),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-2/deny", strings.NewReader(`{"token":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	stored, err := store.GetRequest("req-2")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusPending {
		t.Fatalf("expected pending, got %q", stored.Status())
	}
}

func TestRequestPageIncludesTokenFieldForDenyAction(t *testing.T) {
	srv := NewServer(Dependencies{
		Store: newMemoryStore([]model.Request{
			model.NewRequest(
				"req-4",
				time.Date(2026, 4, 12, 5, 15, 0, 0, time.UTC),
				model.Requester{Username: "rijuyuezhu"},
				model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
			),
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/requests/req-4", nil)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<form method="post" action="/api/requests/req-4/deny">`) {
		t.Fatalf("expected deny form to be rendered")
	}
	if !strings.Contains(body, `<form method="post" action="/api/requests/req-4/deny">
    <label>Token <input name="token" /></label>
    <button type="submit">Deny</button>
  </form>`) {
		t.Fatalf("expected deny flow to render a token input")
	}
}

func TestPendingPageShowsQueuedRequest(t *testing.T) {
	srv := NewServer(Dependencies{
		Store: newMemoryStore([]model.Request{
			model.NewRequest(
				"req-3",
				time.Date(2026, 4, 12, 5, 10, 0, 0, time.UTC),
				model.Requester{Username: "rijuyuezhu"},
				model.Command{ResolvedPath: "/usr/bin/pacman", Argv: []string{"/usr/bin/pacman", "-Syu"}, Cwd: "/tmp"},
			),
		}),
		Templates: testTemplates(t),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "req-3") {
		t.Fatalf("expected pending request to be rendered")
	}
	if !strings.Contains(w.Body.String(), "/usr/bin/pacman") {
		t.Fatalf("expected command path to be rendered")
	}
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()

	return template.Must(template.New("index.html").Parse(`{{define "index.html"}}{{range .Pending}}{{.ID}} {{.Command.ResolvedPath}}{{end}}{{end}}` +
		`{{define "request.html"}}{{.ID}} {{.Status}} {{.RequestedBy.Username}}{{end}}`))
}

type memoryStore struct {
	requests map[string]model.Request
	ordered  []string
}

func newMemoryStore(requests []model.Request) *memoryStore {
	store := &memoryStore{requests: make(map[string]model.Request, len(requests))}
	for _, req := range requests {
		store.requests[req.ID()] = req
		store.ordered = append(store.ordered, req.ID())
	}
	return store
}

func (s *memoryStore) ListPendingRequests() ([]model.Request, error) {
	var pending []model.Request
	for _, id := range s.ordered {
		req := s.requests[id]
		if req.Status() == model.StatusPending {
			pending = append(pending, req)
		}
	}
	return pending, nil
}

func (s *memoryStore) ListRecentRequests() ([]model.Request, error) {
	var recent []model.Request
	for _, id := range s.ordered {
		req := s.requests[id]
		if req.Status() != model.StatusPending {
			recent = append(recent, req)
		}
	}
	return recent, nil
}

func (s *memoryStore) GetRequest(id string) (model.Request, error) {
	req, ok := s.requests[id]
	if !ok {
		return model.Request{}, errors.New("request not found")
	}
	return req, nil
}

func (s *memoryStore) ApproveRequest(id string) (model.Request, error) {
	req, err := s.GetRequest(id)
	if err != nil {
		return model.Request{}, err
	}
	next, err := req.Transition(model.StatusApproved)
	if err != nil {
		return model.Request{}, err
	}
	s.requests[id] = next
	return next, nil
}

func (s *memoryStore) DenyRequest(id string) (model.Request, error) {
	req, err := s.GetRequest(id)
	if err != nil {
		return model.Request{}, err
	}
	next, err := req.Transition(model.StatusDenied)
	if err != nil {
		return model.Request{}, err
	}
	s.requests[id] = next
	return next, nil
}
