package approverd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const askpassConsumeTokenHeader = "X-Websudo-Askpass-Token"

type askpassCreateResponse struct {
	AskpassRequest
	ConsumeToken string `json:"consumeToken"`
}

func (s *Server) handleAskpassCreate(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/askpass" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.askpassStore == nil {
		http.Error(w, "askpass store not configured", http.StatusInternalServerError)
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req := s.askpassStore.Create(body.Prompt)
	consumeToken, err := s.askpassStore.ConsumeToken(req.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, askpassCreateResponse{AskpassRequest: req, ConsumeToken: consumeToken})
}

func (s *Server) handleAskpassAction(w http.ResponseWriter, r *http.Request) {
	if s.askpassStore == nil {
		http.Error(w, "askpass store not configured", http.StatusInternalServerError)
		return
	}
	s.expireAskpassRequests()

	if r.Method == http.MethodGet {
		if !s.requireSession(w, r) {
			return
		}
		id, ok := requestIDFromPath(r.URL.Path, "/api/askpass/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		req, err := s.askpassStore.Get(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id, action, ok := askpassActionFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if action == "consume" {
		password, err := s.askpassStore.Consume(id, r.Header.Get(askpassConsumeTokenHeader))
		if err != nil {
			w.WriteHeader(askpassConsumeStatus(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"password": password})
		return
	}

	if !s.requireSession(w, r) {
		return
	}
	if !isJSONRequest(r) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	switch action {
	case "complete":
		password, err := askpassPassword(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := s.askpassStore.Complete(id, password); err != nil {
			w.WriteHeader(askpassWriteStatus(err))
			return
		}
	case "deny":
		if _, err := s.askpassStore.Deny(id); err != nil {
			w.WriteHeader(askpassWriteStatus(err))
			return
		}
	default:
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleAskpassPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.askpassStore == nil {
		http.Error(w, "askpass store not configured", http.StatusInternalServerError)
		return
	}
	s.expireAskpassRequests()

	id, ok := requestIDFromPath(r.URL.Path, "/askpass/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	req, err := s.askpassStore.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "askpass.html", req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func askpassActionFromPath(path string) (string, string, bool) {
	if !strings.HasPrefix(path, "/api/askpass/") {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/askpass/"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func askpassPassword(r *http.Request) (string, error) {
	if isJSONRequest(r) {
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return "", err
		}
		return body.Password, nil
	}

	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return r.Form.Get("password"), nil
}

func askpassWriteStatus(err error) int {
	if strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	return http.StatusConflict
}

func askpassConsumeStatus(err error) int {
	message := err.Error()
	switch {
	case errors.Is(err, errInvalidAskpassConsumeToken):
		return http.StatusForbidden
	case strings.Contains(message, "not found"):
		return http.StatusNotFound
	case strings.Contains(message, string(AskpassPending)):
		return http.StatusConflict
	case strings.Contains(message, string(AskpassDenied)), strings.Contains(message, string(AskpassExpired)):
		return http.StatusGone
	default:
		return http.StatusConflict
	}
}

func (s *Server) expireAskpassRequests() {
	if s.config.ApprovalTimeoutSeconds <= 0 || s.askpassStore == nil {
		return
	}
	cutoff := time.Now().Add(-time.Duration(s.config.ApprovalTimeoutSeconds) * time.Second).UTC()
	s.askpassStore.ExpireBefore(cutoff)
}
