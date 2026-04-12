package approverd

import (
	"context"
	"encoding/json"
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

func TestCreateRequestAPIStoresFrozenRequest(t *testing.T) {
	store := newMemoryStore(nil)
	srv := NewServer(Dependencies{
		Config:    config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:     store,
		Templates: testTemplates(t),
	})

	body := strings.NewReader(`{"id":"req-create","createdAt":"2026-04-12T06:45:00Z","requestedBy":{"username":"rijuyuezhu"},"command":{"resolvedPath":"/usr/bin/true","argv":["/usr/bin/true"],"cwd":"/tmp"},"status":"pending"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/requests", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	stored, err := store.GetRequest("req-create")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Command().ResolvedPath != "/usr/bin/true" {
		t.Fatalf("resolved path = %q, want %q", stored.Command().ResolvedPath, "/usr/bin/true")
	}
}

func TestCreateRequestAPIIgnoresForgedLifecycleFields(t *testing.T) {
	store := newMemoryStore(nil)
	srv := NewServer(Dependencies{
		Config:    config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:     store,
		Templates: testTemplates(t),
	})

	body := strings.NewReader(`{"id":"req-forged","createdAt":"2026-04-12T06:46:00Z","requestedBy":{"username":"rijuyuezhu"},"command":{"resolvedPath":"/usr/bin/true","argv":["/usr/bin/true"],"cwd":"/tmp"},"status":"succeeded","result":{"exitCode":0,"stdout":"forged"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/requests", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	stored, err := store.GetRequest("req-forged")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusPending {
		t.Fatalf("status = %q, want %q", stored.Status(), model.StatusPending)
	}
	if stored.Result() != nil {
		t.Fatalf("result = %#v, want nil", stored.Result())
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["status"] != string(model.StatusPending) {
		t.Fatalf("payload status = %#v, want %q", payload["status"], model.StatusPending)
	}
	if _, ok := payload["result"]; ok {
		t.Fatalf("payload result = %#v, want omitted", payload["result"])
	}
}

func TestApproveHandlerExecutesRequestAndStoresResult(t *testing.T) {
	store := newMemoryStore([]model.Request{
		model.NewRequest(
			"req-exec",
			time.Date(2026, 4, 12, 7, 0, 0, 0, time.UTC),
			model.Requester{Username: "rijuyuezhu"},
			model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		),
	})
	srv := NewServer(Dependencies{
		Config:    config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:     store,
		Templates: testTemplates(t),
		Executor:  fakeExecutor{result: model.Result{ExitCode: 0, Stdout: "ok"}},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-exec/approve", strings.NewReader(`{"token":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	stored, err := store.GetRequest("req-exec")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusSucceeded {
		t.Fatalf("status = %q, want %q", stored.Status(), model.StatusSucceeded)
	}
	if stored.Result() == nil || stored.Result().Stdout != "ok" {
		t.Fatalf("result = %#v, want persisted execution result", stored.Result())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/requests/req-exec", nil)
	getW := httptest.NewRecorder()

	srv.Routes().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", getW.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.Unmarshal(getW.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["status"] != string(model.StatusSucceeded) {
		t.Fatalf("payload status = %#v, want %q", payload["status"], model.StatusSucceeded)
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

type fakeExecutor struct {
	result model.Result
	err    error
}

func (f fakeExecutor) Execute(_ context.Context, _ model.Command) (model.Result, error) {
	return f.result, f.err
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

func (s *memoryStore) CreateRequest(req model.Request) error {
	if _, exists := s.requests[req.ID()]; exists {
		return errors.New("request already exists")
	}
	s.requests[req.ID()] = req
	s.ordered = append([]string{req.ID()}, s.ordered...)
	return nil
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

func (s *memoryStore) MarkRunning(id string) (model.Request, error) {
	req, err := s.GetRequest(id)
	if err != nil {
		return model.Request{}, err
	}
	next, err := req.Transition(model.StatusRunning)
	if err != nil {
		return model.Request{}, err
	}
	s.requests[id] = next
	return next, nil
}

func (s *memoryStore) CompleteRequest(id string, result model.Result) (model.Request, error) {
	req, err := s.GetRequest(id)
	if err != nil {
		return model.Request{}, err
	}
	next, err := req.WithResult(result)
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
