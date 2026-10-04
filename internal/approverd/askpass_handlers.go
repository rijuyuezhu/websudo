package approverd

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleAskpassAction(w http.ResponseWriter, r *http.Request) {
	if s.askpassStore == nil {
		http.Error(w, "askpass store not configured", http.StatusInternalServerError)
		return
	}

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
	if action != "complete" && action != "deny" {
		http.NotFound(w, r)
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
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := s.askpassStore.Complete(id, body.Password); err != nil {
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

func askpassWriteStatus(err error) int {
	if strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	return http.StatusConflict
}
