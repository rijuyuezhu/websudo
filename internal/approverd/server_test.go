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
	"testing/fstest"
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

func TestApproveHandlerIgnoresTokenWithoutSession(t *testing.T) {
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDenyHandlerIgnoresTokenWithoutSession(t *testing.T) {
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	stored, err := store.GetRequest("req-2")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusPending {
		t.Fatalf("expected pending, got %q", stored.Status())
	}
}

func TestRequestPageServesSPAIndex(t *testing.T) {
	srv := NewServer(Dependencies{
		StaticFS: testStaticFS(),
	})

	req := httptest.NewRequest(http.MethodGet, "/requests/req-4", nil)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `id="app"`) {
		t.Fatalf("expected SPA index to be rendered: %s", w.Body.String())
	}
}

func TestApproveJSONWithSessionReturnsAccepted(t *testing.T) {
	now := time.Date(2026, 4, 12, 7, 30, 0, 0, time.UTC)
	store := newMemoryStore([]model.Request{
		model.NewRequest(
			"req-form-approve",
			now,
			model.Requester{Username: "rijuyuezhu"},
			model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		),
	})
	srv := NewServer(Dependencies{
		Config:       config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:        store,
		Executor:     fakeExecutor{result: model.Result{ExitCode: 0}},
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-form-approve", nil }),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-form-approve/approve", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDenyJSONWithSessionReturnsAccepted(t *testing.T) {
	now := time.Date(2026, 4, 12, 7, 31, 0, 0, time.UTC)
	store := newMemoryStore([]model.Request{
		model.NewRequest(
			"req-form-deny",
			now,
			model.Requester{Username: "rijuyuezhu"},
			model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		),
	})
	srv := NewServer(Dependencies{
		Config:       config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:        store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-form-deny", nil }),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-form-deny/deny", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestRequestActionRejectsNonJSONWithSession(t *testing.T) {
	now := time.Date(2026, 6, 2, 13, 0, 0, 0, time.UTC)
	store := newMemoryStore([]model.Request{
		model.NewRequest("req-form-csrf", now, model.Requester{Username: "rijuyuezhu"}, model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"}),
	})
	srv := NewServer(Dependencies{
		Store:        store,
		Executor:     fakeExecutor{result: model.Result{ExitCode: 0}},
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-request-csrf", nil }),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-form-csrf/approve", strings.NewReader("approve=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
	stored, err := store.GetRequest("req-form-csrf")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusPending {
		t.Fatalf("status = %q, want %q", stored.Status(), model.StatusPending)
	}
}

func TestIndexServesSPAIndex(t *testing.T) {
	srv := NewServer(Dependencies{
		StaticFS: testStaticFS(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `id="app"`) {
		t.Fatalf("expected SPA index to be rendered: %s", w.Body.String())
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
	now := time.Date(2026, 4, 12, 7, 0, 0, 0, time.UTC)
	store := newMemoryStore([]model.Request{
		model.NewRequest(
			"req-exec",
			now,
			model.Requester{Username: "rijuyuezhu"},
			model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		),
	})
	srv := NewServer(Dependencies{
		Config:    config.Config{TokenHashHex: config.MustHashToken("123456")},
		Store:     store,
		Templates: testTemplates(t),
		Executor:  fakeExecutor{result: model.Result{ExitCode: 0, Stdout: "ok"}},
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-exec", nil
		}),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-exec/approve", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, req)
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

func TestGetRequestExpiresStalePendingRequest(t *testing.T) {
	store := newMemoryStore([]model.Request{
		model.NewRequest(
			"req-expire",
			time.Now().Add(-2*time.Minute).UTC(),
			model.Requester{Username: "rijuyuezhu"},
			model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"},
		),
	})
	srv := NewServer(Dependencies{
		Config:    config.Config{ApprovalTimeoutSeconds: 60, TokenHashHex: config.MustHashToken("123456")},
		Store:     store,
		Templates: testTemplates(t),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-expire", nil)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["status"] != string(model.StatusExpired) {
		t.Fatalf("payload status = %#v, want %q", payload["status"], model.StatusExpired)
	}
	stored, err := store.GetRequest("req-expire")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusExpired {
		t.Fatalf("stored status = %q, want %q", stored.Status(), model.StatusExpired)
	}
}

func TestAskpassCreateGetCompleteAndConsume(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-http" })
	srv := NewServer(Dependencies{
		AskpassStore: store,
		Templates:    testTemplates(t),
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-http", nil
		}),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/askpass", strings.NewReader(`{"prompt":"Password:"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createW.Code, http.StatusCreated)
	}
	var createPayload struct {
		ConsumeToken string `json:"consumeToken"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("Unmarshal(create) error = %v", err)
	}
	if createPayload.ConsumeToken == "" {
		t.Fatal("consume token is empty")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-http", nil)
	addSessionCookie(t, srv, getReq)
	getW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getW.Code, http.StatusOK)
	}
	if strings.Contains(getW.Body.String(), "secret") {
		t.Fatalf("GET leaked password: %q", getW.Body.String())
	}
	if strings.Contains(getW.Body.String(), createPayload.ConsumeToken) || strings.Contains(getW.Body.String(), "consumeToken") {
		t.Fatalf("GET leaked consume token: %q", getW.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-http/complete", strings.NewReader(`{"password":"secret"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, completeReq)
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusAccepted {
		t.Fatalf("complete status = %d, want %d", completeW.Code, http.StatusAccepted)
	}
	if strings.Contains(completeW.Body.String(), "secret") {
		t.Fatalf("complete echoed password: %q", completeW.Body.String())
	}

	consumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-http/consume", nil)
	consumeReq.Header.Set("X-Websudo-Askpass-Token", createPayload.ConsumeToken)
	consumeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(consumeW, consumeReq)
	if consumeW.Code != http.StatusOK {
		t.Fatalf("consume status = %d, want %d", consumeW.Code, http.StatusOK)
	}
	var payload map[string]string
	if err := json.Unmarshal(consumeW.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["password"] != "secret" {
		t.Fatalf("password = %q, want secret", payload["password"])
	}

	secondConsume := httptest.NewRecorder()
	srv.Routes().ServeHTTP(secondConsume, consumeReq.Clone(context.Background()))
	if secondConsume.Code != http.StatusNotFound {
		t.Fatalf("second consume status = %d, want %d", secondConsume.Code, http.StatusNotFound)
	}
}

func TestAskpassConsumeRequiresTokenAndDoesNotConsumeOnFailure(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-token-http" })
	srv := NewServer(Dependencies{
		AskpassStore: store,
		Templates:    testTemplates(t),
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-token-http", nil
		}),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/askpass", strings.NewReader(`{"prompt":"Password:"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createW.Code, http.StatusCreated)
	}
	var createPayload struct {
		ConsumeToken string `json:"consumeToken"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("Unmarshal(create) error = %v", err)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-token-http/complete", strings.NewReader(`{"password":"secret"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, completeReq)
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusAccepted {
		t.Fatalf("complete status = %d, want %d", completeW.Code, http.StatusAccepted)
	}

	for _, tc := range []struct {
		name  string
		token string
	}{
		{name: "missing"},
		{name: "wrong", token: "wrong"},
	} {
		consumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-token-http/consume", nil)
		if tc.token != "" {
			consumeReq.Header.Set("X-Websudo-Askpass-Token", tc.token)
		}
		consumeW := httptest.NewRecorder()
		srv.Routes().ServeHTTP(consumeW, consumeReq)
		if consumeW.Code != http.StatusForbidden {
			t.Fatalf("%s token consume status = %d, want %d", tc.name, consumeW.Code, http.StatusForbidden)
		}
		if strings.Contains(consumeW.Body.String(), "secret") {
			t.Fatalf("%s token consume leaked password: %q", tc.name, consumeW.Body.String())
		}
	}

	consumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-token-http/consume", nil)
	consumeReq.Header.Set("X-Websudo-Askpass-Token", createPayload.ConsumeToken)
	consumeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(consumeW, consumeReq)
	if consumeW.Code != http.StatusOK {
		t.Fatalf("valid token consume status = %d, want %d", consumeW.Code, http.StatusOK)
	}
	if !strings.Contains(consumeW.Body.String(), "secret") {
		t.Fatalf("valid token consume response = %q, want password", consumeW.Body.String())
	}
}

func TestAskpassConsumeTokenOnlyReturnedOnCreate(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-token-visibility" })
	srv := NewServer(Dependencies{
		AskpassStore: store,
		Templates:    testTemplates(t),
		StaticFS:     testStaticFS(),
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-token-visibility", nil
		}),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/askpass", strings.NewReader(`{"prompt":"Password:"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createW.Code, http.StatusCreated)
	}
	var createPayload struct {
		ConsumeToken string `json:"consumeToken"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("Unmarshal(create) error = %v", err)
	}
	if createPayload.ConsumeToken == "" {
		t.Fatal("consume token is empty")
	}

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{name: "status", req: httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-token-visibility", nil)},
		{name: "page", req: httptest.NewRequest(http.MethodGet, "/askpass/askpass-token-visibility", nil)},
		{name: "index", req: httptest.NewRequest(http.MethodGet, "/", nil)},
	} {
		if tc.name == "status" {
			addSessionCookie(t, srv, tc.req)
		}
		w := httptest.NewRecorder()
		srv.Routes().ServeHTTP(w, tc.req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", tc.name, w.Code, http.StatusOK)
		}
		body := w.Body.String()
		if strings.Contains(body, createPayload.ConsumeToken) || strings.Contains(body, "consumeToken") {
			t.Fatalf("%s leaked consume token: %q", tc.name, body)
		}
	}
}

func TestAskpassServerConfiguresCompletedExpirationTimeout(t *testing.T) {
	current := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return current }, func() string { return "askpass-config-expire" })
	store.Create("Password:")
	token, err := store.ConsumeToken("askpass-config-expire")
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}
	srv := NewServer(Dependencies{
		Config:       config.Config{ApprovalTimeoutSeconds: 1},
		AskpassStore: store,
		Templates:    testTemplates(t),
	})
	if srv == nil {
		t.Fatal("NewServer() = nil")
	}

	current = current.Add(2 * time.Second)
	completed, err := store.Complete("askpass-config-expire", "secret")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.Status != AskpassExpired {
		t.Fatalf("completed status = %q, want %q", completed.Status, AskpassExpired)
	}

	if _, err := store.Consume("askpass-config-expire", token); err == nil || !strings.Contains(err.Error(), string(AskpassExpired)) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("Consume(expired configured) error = %v, want expired without password", err)
	}
}

func TestAskpassDenyAndPendingConsumeStatus(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	ids := []string{"askpass-pending", "askpass-deny"}
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	store.Create("pending")
	store.Create("deny")
	pendingToken, err := store.ConsumeToken("askpass-pending")
	if err != nil {
		t.Fatalf("ConsumeToken(pending) error = %v", err)
	}
	denyToken, err := store.ConsumeToken("askpass-deny")
	if err != nil {
		t.Fatalf("ConsumeToken(deny) error = %v", err)
	}
	srv := NewServer(Dependencies{
		AskpassStore: store,
		Templates:    testTemplates(t),
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-deny", nil
		}),
	})

	pendingConsume := httptest.NewRecorder()
	pendingConsumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-pending/consume", nil)
	pendingConsumeReq.Header.Set("X-Websudo-Askpass-Token", pendingToken)
	srv.Routes().ServeHTTP(pendingConsume, pendingConsumeReq)
	if pendingConsume.Code != http.StatusConflict {
		t.Fatalf("pending consume status = %d, want %d", pendingConsume.Code, http.StatusConflict)
	}

	denyReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-deny/deny", nil)
	denyReq.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, denyReq)
	denyW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(denyW, denyReq)
	if denyW.Code != http.StatusAccepted {
		t.Fatalf("deny status = %d, want %d", denyW.Code, http.StatusAccepted)
	}

	deniedConsume := httptest.NewRecorder()
	deniedConsumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-deny/consume", nil)
	deniedConsumeReq.Header.Set("X-Websudo-Askpass-Token", denyToken)
	srv.Routes().ServeHTTP(deniedConsume, deniedConsumeReq)
	if deniedConsume.Code != http.StatusGone {
		t.Fatalf("denied consume status = %d, want %d", deniedConsume.Code, http.StatusGone)
	}
}

func TestAskpassBrowserActionRejectsNonJSONWithSession(t *testing.T) {
	now := time.Date(2026, 6, 2, 13, 5, 0, 0, time.UTC)
	askpassStore := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-csrf" })
	askpassStore.Create("Password:")
	srv := NewServer(Dependencies{
		AskpassStore: askpassStore,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-askpass-csrf", nil }),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-csrf/complete", strings.NewReader("password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
	stored, err := askpassStore.Get("askpass-csrf")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status != AskpassPending {
		t.Fatalf("status = %q, want %q", stored.Status, AskpassPending)
	}
}

func TestAskpassPageServesSPAIndex(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: testStaticFS()})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/askpass/askpass-page", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="app"`) {
		t.Fatalf("expected SPA index to be rendered: %s", body)
	}
}

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
	}
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()

	return template.Must(template.New("index.html").Parse(`{{define "index.html"}}{{range .Pending}}{{.ID}} {{.Command.ResolvedPath}}{{end}}{{range .AskpassPending}}{{.ID}} {{.Prompt}}{{end}}{{end}}` +
		`{{define "request.html"}}{{.ID}} {{.Status}} {{.RequestedBy.Username}}{{end}}` +
		`{{define "askpass.html"}}{{.ID}} {{.Prompt}}<form method="post" action="/api/askpass/{{.ID}}/complete"><input type="password" name="password" /></form>{{end}}`))
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

func (s *memoryStore) ExpirePendingRequests(before time.Time) (int, error) {
	expired := 0
	for _, id := range s.ordered {
		req := s.requests[id]
		if req.Status() != model.StatusPending || req.CreatedAt().After(before) {
			continue
		}
		next, err := req.Transition(model.StatusExpired)
		if err != nil {
			return 0, err
		}
		s.requests[id] = next
		expired++
	}
	return expired, nil
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

func TestDashboardRequiresSession(t *testing.T) {
	srv := NewServer(Dependencies{
		Store:        newMemoryStore(nil),
		AskpassStore: newAskpassStoreForTest(time.Now, func() string { return "askpass-dashboard" }),
	})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDashboardReturnsListsWithSession(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	askpassStore := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-dashboard" })
	askpassStore.Create("Password:")
	srv := NewServer(Dependencies{
		Store: newMemoryStore([]model.Request{
			model.NewRequest("req-dashboard", now, model.Requester{Username: "rijuyuezhu"}, model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"}),
		}),
		AskpassStore: askpassStore,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-dashboard", nil }),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "askpass-dashboard") || !strings.Contains(body, "req-dashboard") {
		t.Fatalf("dashboard response missing expected ids: %s", body)
	}
}

func TestDashboardReturnsEmptyArraysWithSession(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 10, 0, 0, time.UTC)
	srv := NewServer(Dependencies{
		Store:        newMemoryStore(nil),
		AskpassStore: newAskpassStoreForTest(func() time.Time { return now }, func() string { return "unused" }),
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-empty-dashboard", nil }),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{`"askpassPending":[]`, `"pending":[]`, `"recent":[]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard response missing %s: %s", want, body)
		}
	}
}

func TestBrowserRequestDetailRequiresSession(t *testing.T) {
	srv := NewServer(Dependencies{
		Store: newMemoryStore([]model.Request{
			model.NewRequest("req-browser-detail", time.Now().UTC(), model.Requester{Username: "rijuyuezhu"}, model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"}),
		}),
	})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/browser/requests/req-browser-detail", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestBrowserRequestDetailReturnsRequestWithSession(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	srv := NewServer(Dependencies{
		Store: newMemoryStore([]model.Request{
			model.NewRequest("req-browser-detail", now, model.Requester{Username: "rijuyuezhu"}, model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"}),
		}),
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-browser-detail", nil }),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/browser/requests/req-browser-detail", nil)
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "req-browser-detail") {
		t.Fatalf("response missing request id: %s", w.Body.String())
	}
}

func TestBrowserRequestDetailWithoutStoreReturnsNotFound(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	srv := NewServer(Dependencies{
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-no-store-detail", nil }),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/browser/requests/req-no-store", nil)
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestApproveActionRequiresSession(t *testing.T) {
	srv := NewServer(Dependencies{
		Store: newMemoryStore([]model.Request{
			model.NewRequest("req-no-session", time.Now().UTC(), model.Requester{Username: "rijuyuezhu"}, model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"}),
		}),
		Executor: fakeExecutor{result: model.Result{ExitCode: 0}},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-no-session/approve", nil)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestApproveActionUsesSessionWithoutToken(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	store := newMemoryStore([]model.Request{
		model.NewRequest("req-session-approve", now, model.Requester{Username: "rijuyuezhu"}, model.Command{ResolvedPath: "/usr/bin/true", Argv: []string{"/usr/bin/true"}, Cwd: "/tmp"}),
	})
	srv := NewServer(Dependencies{
		Store:        store,
		Executor:     fakeExecutor{result: model.Result{ExitCode: 0}},
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-approve", nil }),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-session-approve/approve", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	stored, err := store.GetRequest("req-session-approve")
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if stored.Status() != model.StatusSucceeded {
		t.Fatalf("status = %q, want %q", stored.Status(), model.StatusSucceeded)
	}
}

func TestAskpassCompleteRequiresSessionButConsumeUsesToken(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	askpassStore := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-auth" })
	askpassStore.newToken = func() string { return "consume-token" }
	askpassStore.Create("Password:")
	srv := NewServer(Dependencies{
		AskpassStore: askpassStore,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-askpass", nil }),
	})

	unauthComplete := httptest.NewRecorder()
	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/complete", strings.NewReader(`{"password":"secret"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	srv.Routes().ServeHTTP(unauthComplete, completeReq)
	if unauthComplete.Code != http.StatusUnauthorized {
		t.Fatalf("complete status = %d, want %d", unauthComplete.Code, http.StatusUnauthorized)
	}

	authComplete := httptest.NewRecorder()
	authReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/complete", strings.NewReader(`{"password":"secret"}`))
	authReq.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, authReq)
	srv.Routes().ServeHTTP(authComplete, authReq)
	if authComplete.Code != http.StatusAccepted {
		t.Fatalf("auth complete status = %d, want %d", authComplete.Code, http.StatusAccepted)
	}

	consume := httptest.NewRecorder()
	consumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/consume", nil)
	consumeReq.Header.Set(askpassConsumeTokenHeader, "consume-token")
	srv.Routes().ServeHTTP(consume, consumeReq)
	if consume.Code != http.StatusOK {
		t.Fatalf("consume status = %d, want %d", consume.Code, http.StatusOK)
	}
	if !strings.Contains(consume.Body.String(), "secret") {
		t.Fatalf("consume body missing password: %s", consume.Body.String())
	}
}

func addSessionCookie(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()
	id, _, err := srv.sessions.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: id})
}
