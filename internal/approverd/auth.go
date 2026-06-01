package approverd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const sessionCookieName = "websudo_session"

type PasswordVerifier interface {
	VerifyPassword(context.Context, string) error
}

type passwordCommandRunner func(context.Context, string, []string, string) error

type SudoPasswordVerifier struct {
	SudoPath string
	Timeout  time.Duration
	Run      passwordCommandRunner
}

func (v SudoPasswordVerifier) VerifyPassword(ctx context.Context, password string) error {
	sudoPath := strings.TrimSpace(v.SudoPath)
	if sudoPath == "" {
		sudoPath = "/usr/bin/sudo"
	}
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	run := v.Run
	if run == nil {
		run = runPasswordCommand
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return run(ctx, sudoPath, []string{"-k", "-S", "-p", "", "-v"}, password+"\n")
}

func runPasswordCommand(ctx context.Context, name string, args []string, stdin string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.New("password rejected")
	}
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/login" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.passwordVerifier.VerifyPassword(r.Context(), body.Password); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id, expiresAt, err := s.sessions.Create()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, sessionCookie(id, expiresAt))
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "expiresAt": expiresAt})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/session" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.hasSession(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/logout" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	http.SetCookie(w, expiredSessionCookie())
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) bool {
	if s.hasSession(r) {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	return false
}

func (s *Server) hasSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return s.sessions.Valid(cookie.Value)
}

func sessionCookie(value string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionTTL / time.Second),
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func expiredSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}
