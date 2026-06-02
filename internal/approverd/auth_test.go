package approverd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"websudo/internal/config"
)

type fakePasswordVerifier struct {
	wantPassword string
	err          error
	called       bool
}

func (f *fakePasswordVerifier) VerifyPassword(_ context.Context, password string) error {
	f.called = true
	if password != f.wantPassword {
		return errors.New("unexpected password")
	}
	return f.err
}

func TestLoginSetsSessionCookie(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	verifier := &fakePasswordVerifier{wantPassword: "machine-secret"}
	srv := NewServer(Dependencies{
		Config:           config.Config{SudoPath: "/usr/bin/sudo"},
		PasswordVerifier: verifier,
		SessionStore:     newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-login", nil }),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"machine-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !verifier.called {
		t.Fatal("password verifier was not called")
	}
	cookie := findCookie(t, w.Result().Cookies(), sessionCookieName)
	if cookie.Value != "session-login" {
		t.Fatalf("cookie value = %q, want %q", cookie.Value, "session-login")
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("Path = %q, want /", cookie.Path)
	}
	if cookie.MaxAge != int((72*time.Hour)/time.Second) {
		t.Fatalf("MaxAge = %d, want 259200", cookie.MaxAge)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	verifier := &fakePasswordVerifier{wantPassword: "wrong", err: errors.New("rejected")}
	srv := NewServer(Dependencies{PasswordVerifier: verifier})

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("login failure set cookies: %#v", w.Result().Cookies())
	}
}

func TestLoginRequiresJSONContentType(t *testing.T) {
	verifier := &fakePasswordVerifier{wantPassword: "machine-secret"}
	srv := NewServer(Dependencies{PasswordVerifier: verifier})

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"machine-secret"}`))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
	if verifier.called {
		t.Fatal("password verifier should not be called for unsupported media type")
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("unsupported media type set cookies: %#v", w.Result().Cookies())
	}
}

func TestLoginRejectsJSONPrefixContentType(t *testing.T) {
	verifier := &fakePasswordVerifier{wantPassword: "machine-secret"}
	srv := NewServer(Dependencies{PasswordVerifier: verifier})

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"machine-secret"}`))
	req.Header.Set("Content-Type", "application/jsonp")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
	if verifier.called {
		t.Fatal("password verifier should not be called for JSON-prefix media type")
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("JSON-prefix media type set cookies: %#v", w.Result().Cookies())
	}
}

func TestLoginRejectsTrailingJSON(t *testing.T) {
	verifier := &fakePasswordVerifier{wantPassword: "machine-secret"}
	srv := NewServer(Dependencies{PasswordVerifier: verifier})

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"machine-secret"} {}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if verifier.called {
		t.Fatal("password verifier should not be called for malformed JSON body")
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("malformed login body set cookies: %#v", w.Result().Cookies())
	}
}

func TestSessionEndpointReflectsAuthState(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	store := newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-active", nil })
	if _, _, err := store.Create(); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	srv := NewServer(Dependencies{SessionStore: store})

	unauth := httptest.NewRecorder()
	srv.Routes().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want %d", unauth.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	authReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-active"})
	auth := httptest.NewRecorder()
	srv.Routes().ServeHTTP(auth, authReq)
	if auth.Code != http.StatusOK {
		t.Fatalf("auth status = %d, want %d", auth.Code, http.StatusOK)
	}
}

func TestLogoutDeletesSessionAndExpiresCookie(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	store := newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-logout", nil })
	if _, _, err := store.Create(); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	srv := NewServer(Dependencies{SessionStore: store})

	req := httptest.NewRequest(http.MethodPost, "/api/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-logout"})
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if store.Valid("session-logout") {
		t.Fatal("logout should delete server-side session")
	}
	cookie := findCookie(t, w.Result().Cookies(), sessionCookieName)
	if cookie.MaxAge != -1 || cookie.Value != "" {
		t.Fatalf("expired cookie = %#v, want empty value and MaxAge -1", cookie)
	}
}

func TestLogoutRejectsNonJSONContentType(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 5, 0, 0, time.UTC)
	store := newSessionStoreForTest(72*time.Hour, func() time.Time { return now }, func() (string, error) { return "session-logout-csrf", nil })
	if _, _, err := store.Create(); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	srv := NewServer(Dependencies{SessionStore: store})

	req := httptest.NewRequest(http.MethodPost, "/api/logout", strings.NewReader("logout=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-logout-csrf"})
	w := httptest.NewRecorder()

	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
	if !store.Valid("session-logout-csrf") {
		t.Fatal("non-JSON logout should not delete server-side session")
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("non-JSON logout set cookies: %#v", w.Result().Cookies())
	}
}

func TestSudoPasswordVerifierForcesPasswordCheck(t *testing.T) {
	type runCall struct {
		name  string
		args  []string
		stdin string
	}
	var calls []runCall
	verifier := SudoPasswordVerifier{
		SudoPath: "/usr/bin/sudo",
		Run: func(_ context.Context, name string, args []string, stdin string) error {
			calls = append(calls, runCall{
				name:  name,
				args:  append([]string(nil), args...),
				stdin: stdin,
			})
			if len(calls) == 1 {
				return errors.New("password required")
			}
			return nil
		},
	}

	if err := verifier.VerifyPassword(context.Background(), "secret"); err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	if calls[0].name != "/usr/bin/sudo" {
		t.Fatalf("probe command = %q, want /usr/bin/sudo", calls[0].name)
	}
	wantProbeArgs := []string{"-k", "-n", "-v"}
	if !reflect.DeepEqual(calls[0].args, wantProbeArgs) {
		t.Fatalf("probe args = %#v, want %#v", calls[0].args, wantProbeArgs)
	}
	if calls[0].stdin != "" {
		t.Fatalf("probe stdin = %q, want empty", calls[0].stdin)
	}
	if calls[1].name != "/usr/bin/sudo" {
		t.Fatalf("password command = %q, want /usr/bin/sudo", calls[1].name)
	}
	wantArgs := []string{"-k", "-S", "-p", "", "-v"}
	if !reflect.DeepEqual(calls[1].args, wantArgs) {
		t.Fatalf("password args = %#v, want %#v", calls[1].args, wantArgs)
	}
	if calls[1].stdin != "secret\n" {
		t.Fatalf("password stdin = %q, want password plus newline", calls[1].stdin)
	}
}

func TestSudoPasswordVerifierRejectsPasswordlessSudo(t *testing.T) {
	calls := 0
	verifier := SudoPasswordVerifier{
		SudoPath: "/usr/bin/sudo",
		Run: func(_ context.Context, _ string, args []string, stdin string) error {
			calls++
			wantArgs := []string{"-k", "-n", "-v"}
			if !reflect.DeepEqual(args, wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, wantArgs)
			}
			if stdin != "" {
				t.Fatalf("stdin = %q, want empty", stdin)
			}
			return nil
		},
	}

	err := verifier.VerifyPassword(context.Background(), "anything")
	if err == nil {
		t.Fatal("VerifyPassword() error = nil, want error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q not found in %#v", name, cookies)
	return nil
}
