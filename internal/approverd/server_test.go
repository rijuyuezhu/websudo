package approverd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAskpassLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-http" })
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-http", nil
		}),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/askpass", strings.NewReader(`{"prompt":"Password:"}`))
	createW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createW.Code, http.StatusCreated)
	}
	var created askpassCreateResponse
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(create) error = %v", err)
	}
	if created.ID != "askpass-http" || created.Prompt != "Password:" {
		t.Fatalf("created = %#v, want askpass id and prompt", created)
	}
	if created.ConsumeToken == "" {
		t.Fatal("consume token is empty")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-http", nil)
	addSessionCookie(t, srv, getReq)
	getW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getW.Code, http.StatusOK)
	}
	if strings.Contains(getW.Body.String(), created.ConsumeToken) {
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

	consumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-http/consume", nil)
	consumeReq.Header.Set(askpassConsumeTokenHeader, created.ConsumeToken)
	consumeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(consumeW, consumeReq)
	if consumeW.Code != http.StatusOK {
		t.Fatalf("consume status = %d, want %d", consumeW.Code, http.StatusOK)
	}
	if !strings.Contains(consumeW.Body.String(), "secret") {
		t.Fatalf("consume response = %q, want password", consumeW.Body.String())
	}
}

func TestAskpassConsumeRequiresToken(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-token-http" })
	store.newToken = func() string { return "consume-token" }
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-token-http", nil
		}),
	})
	store.Create("Password:")
	if _, err := store.Complete("askpass-token-http", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
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
			consumeReq.Header.Set(askpassConsumeTokenHeader, tc.token)
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
}

func TestAskpassActionsRequireBrowserSession(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-auth" })
	store.Create("Password:")
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-askpass", nil }),
	})

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/complete", strings.NewReader(`{"password":"secret"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusUnauthorized {
		t.Fatalf("complete without session status = %d, want %d", completeW.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/complete", strings.NewReader(`{"password":"secret"}`))
	authReq.Header.Set("Content-Type", "application/json")
	addSessionCookie(t, srv, authReq)
	authW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(authW, authReq)
	if authW.Code != http.StatusAccepted {
		t.Fatalf("complete with session status = %d, want %d", authW.Code, http.StatusAccepted)
	}
}

func TestDashboardRequiresSession(t *testing.T) {
	srv := NewServer(Dependencies{
		SessionStore: newSessionStoreForTest(72*time.Hour, time.Now, func() (string, error) { return "session-dashboard", nil }),
	})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDashboardReturnsAskpassPromptsWithSession(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	askpassStore := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-dashboard" })
	askpassStore.Create("Password:")
	srv := NewServer(Dependencies{
		AskpassStore: askpassStore,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-dashboard", nil }),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	addSessionCookie(t, srv, req)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "askpass-dashboard") {
		t.Fatalf("dashboard body = %q, want askpass prompt", body)
	}
	for _, notWant := range []string{`"pending":`, `"recent":`, "req-"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("dashboard body = %q, contains removed command request field %q", body, notWant)
		}
	}
}

func addSessionCookie(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()
	id, expiresAt, err := srv.sessions.Create()
	if err != nil {
		t.Fatalf("Create session error = %v", err)
	}
	req.AddCookie(sessionCookie(id, expiresAt))
}
