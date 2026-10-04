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
	request := store.Create("Password:", AskpassProvenance{})
	result, err := store.Result(request.ID)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	srv := NewServer(Dependencies{
		AskpassStore: store,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-askpass-browser", nil
		}),
	})

	getReq := httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-browser", nil)
	addSessionCookie(t, srv, getReq)
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
	addSessionCookie(t, srv, completeReq)
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
	store.Create("Password:", AskpassProvenance{})
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
	addSessionCookie(t, srv, authReq)
	authW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(authW, authReq)
	if authW.Code != http.StatusAccepted {
		t.Fatalf("complete with session status = %d, want %d", authW.Code, http.StatusAccepted)
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
	askpassStore := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-dashboard" })
	askpassStore.Create("Password:", AskpassProvenance{
		Command: []string{"/usr/bin/id", "-u"},
		CWD:     "/home/alice/project",
	})
	srv := NewServer(Dependencies{
		AskpassStore: askpassStore,
		SessionStore: newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) {
			return "session-dashboard", nil
		}),
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
	for _, want := range []string{"/usr/bin/id", "/home/alice/project"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard body = %q, want provenance %q", body, want)
		}
	}
	for _, notWant := range []string{"\"pending\":", "\"recent\":", "req-"} {
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
