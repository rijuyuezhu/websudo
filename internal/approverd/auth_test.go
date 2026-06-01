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

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
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

func TestSudoPasswordVerifierForcesPasswordCheck(t *testing.T) {
	var gotName string
	var gotArgs []string
	var gotInput string
	verifier := SudoPasswordVerifier{
		SudoPath: "/usr/bin/sudo",
		Run: func(_ context.Context, name string, args []string, stdin string) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			gotInput = stdin
			return nil
		},
	}

	if err := verifier.VerifyPassword(context.Background(), "secret"); err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if gotName != "/usr/bin/sudo" {
		t.Fatalf("command = %q, want /usr/bin/sudo", gotName)
	}
	wantArgs := []string{"-k", "-S", "-p", "", "-v"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
	if gotInput != "secret\n" {
		t.Fatalf("stdin = %q, want password plus newline", gotInput)
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
