package approverd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrowserAskpassLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-browser" })
	_, result := store.Create("Password:", AskpassProvenance{})
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-browser", nil
		}),
	})

	getReq := httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-browser", nil)
	addSessionAuthorization(t, srv, getReq)
	getW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getW.Code, http.StatusOK)
	}
	if !strings.Contains(getW.Body.String(), "\"prompt\":\"Password:\"") {
		t.Fatalf("GET body = %q, want prompt", getW.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-browser/complete", strings.NewReader("{\"password\":\"secret\"}"))
	completeReq.Header.Set("Content-Type", "application/json")
	addSessionAuthorization(t, srv, completeReq)
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusAccepted {
		t.Fatalf("complete status = %d, want %d", completeW.Code, http.StatusAccepted)
	}

	outcome := <-result
	if outcome.status != AskpassCompleted || outcome.password != "secret" {
		t.Fatalf("outcome = %#v, want completed secret", outcome)
	}
}

func TestCompletedAskpassRemainsReadableButCannotBeCompletedAgain(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-terminal-browser" })
	_, _ = store.Create("Password:", AskpassProvenance{})
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-terminal-browser", nil
		}),
	})

	if _, err := store.Complete("askpass-terminal-browser", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-terminal-browser", nil)
	addSessionAuthorization(t, srv, getReq)
	getW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK || !strings.Contains(getW.Body.String(), `"status":"completed"`) {
		t.Fatalf("terminal GET status/body = %d %q", getW.Code, getW.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-terminal-browser/complete", strings.NewReader(`{"password":"again"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	addSessionAuthorization(t, srv, completeReq)
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusConflict {
		t.Fatalf("repeated complete status = %d, want %d", completeW.Code, http.StatusConflict)
	}
}

func TestAskpassCreateAndConsumeHTTPRoutesAreRemoved(t *testing.T) {
	srv := NewServer(Dependencies{})

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/askpass", strings.NewReader("{\"prompt\":\"Password:\"}")),
		httptest.NewRequest(http.MethodPost, "/api/askpass/example/consume", nil),
	} {
		w := httptest.NewRecorder()
		srv.Routes().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want %d", req.Method, req.URL.Path, w.Code, http.StatusNotFound)
		}
	}
}

func TestAskpassActionsRequireBrowserSession(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-auth" })
	_, _ = store.Create("Password:", AskpassProvenance{})
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass", nil
		}),
	})

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/complete", strings.NewReader("{\"password\":\"secret\"}"))
	completeReq.Header.Set("Content-Type", "application/json")
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusUnauthorized {
		t.Fatalf("complete without session status = %d, want %d", completeW.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-auth/complete", strings.NewReader("{\"password\":\"secret\"}"))
	authReq.Header.Set("Content-Type", "application/json")
	addSessionAuthorization(t, srv, authReq)
	authW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(authW, authReq)
	if authW.Code != http.StatusAccepted {
		t.Fatalf("complete with session status = %d, want %d", authW.Code, http.StatusAccepted)
	}
}

func TestAskpassCompletionRejectsFormBody(t *testing.T) {
	store := newAskpassStoreForTest(time.Now, func() string { return "askpass-json-only" })
	_, _ = store.Create("Password:", AskpassProvenance{})
	srv := NewServer(Dependencies{AskpassStore: store})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/askpass/askpass-json-only/complete",
		strings.NewReader("password=secret"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionAuthorization(t, srv, req)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("complete form status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}

	pending, err := store.Get("askpass-json-only")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if pending.Status != AskpassPending {
		t.Fatalf("request status = %q, want pending", pending.Status)
	}
}

func TestDashboardRequiresSession(t *testing.T) {
	srv := NewServer(Dependencies{
		SessionStore: newSessionStoreForTest(72*time.Hour, time.Now, func() (string, error) {
			return "session-dashboard", nil
		}),
	})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDashboardReturnsAskpassPromptsWithSession(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	ids := []string{"askpass-pending", "askpass-recent"}
	askpassStore := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	_, _ = askpassStore.Create("Password:", AskpassProvenance{
		Command: []string{"/usr/bin/id", "-u"},
		CWD:     "/home/alice/project",
	})
	_, _ = askpassStore.Create("Password:", AskpassProvenance{
		Command: []string{"/usr/bin/systemctl", "restart", "example.service"},
		CWD:     "/home/alice/admin",
	})
	now = now.Add(time.Second)
	if _, err := askpassStore.Deny("askpass-recent"); err != nil {
		t.Fatalf("Deny(recent) error = %v", err)
	}
	srv := NewServer(Dependencies{
		AskpassStore: askpassStore,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-dashboard", nil
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	addSessionAuthorization(t, srv, req)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{"\"askpassPending\"", "askpass-pending", "\"askpassRecent\"", "askpass-recent", "\"status\":\"denied\"", "\"finishedAt\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard body = %q, want %q", body, want)
		}
	}
	for _, want := range []string{"/usr/bin/id", "/home/alice/project", "/usr/bin/systemctl", "/home/alice/admin"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard body = %q, want provenance %q", body, want)
		}
	}
	for _, notWant := range []string{"\"password\":", "req-"} {
		if strings.Contains(body, notWant) {
			t.Fatalf("dashboard body = %q, contains secret/removed field %q", body, notWant)
		}
	}
}

func addSessionAuthorization(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()
	id, _, err := srv.sessions.Create()
	if err != nil {
		t.Fatalf("Create session error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+id)
}
