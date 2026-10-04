package approverd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

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
	if err := run(ctx, sudoPath, []string{"-k", "-n", "-v"}, ""); err == nil {
		return errors.New("password verification unavailable")
	}
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
	if !isJSONRequest(r) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var trailing struct{}
	if err := dec.Decode(&trailing); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.passwordVerifier.VerifyPassword(r.Context(), body.Password); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id, _, err := s.sessions.Create()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Token string `json:"token"`
	}{Token: id})
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
	w.WriteHeader(http.StatusNoContent)
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
	if !isJSONRequest(r) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	if id, ok := sessionToken(r); ok {
		s.sessions.Delete(id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) bool {
	if s.hasSession(r) {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	return false
}

func (s *Server) hasSession(r *http.Request) bool {
	id, ok := sessionToken(r)
	return ok && s.sessions.Valid(id)
}

func sessionToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	return token, token != ""
}
