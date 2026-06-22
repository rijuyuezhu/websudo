package approverd

import (
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"time"

	"websudo/internal/config"
)

type Dependencies struct {
	Config           config.Config
	AskpassStore     *AskpassStore
	PasswordVerifier PasswordVerifier
	SessionStore     *SessionStore
	StaticFS         fs.FS
}

type Server struct {
	config           config.Config
	askpassStore     *AskpassStore
	passwordVerifier PasswordVerifier
	sessions         *SessionStore
	staticFS         fs.FS
}

func NewServer(dep Dependencies) *Server {
	askpassStore := dep.AskpassStore
	if askpassStore == nil {
		askpassStore = NewAskpassStore()
	}
	askpassStore.setExpirationTimeout(time.Duration(dep.Config.ApprovalTimeoutSeconds) * time.Second)
	passwordVerifier := dep.PasswordVerifier
	if passwordVerifier == nil {
		passwordVerifier = SudoPasswordVerifier{SudoPath: dep.Config.SudoPath}
	}
	sessions := dep.SessionStore
	if sessions == nil {
		sessions = NewSessionStore()
	}
	staticFS := dep.StaticFS
	if staticFS == nil {
		staticFS = embeddedFrontendFS()
	}

	return &Server{
		config:           dep.Config,
		askpassStore:     askpassStore,
		passwordVerifier: passwordVerifier,
		sessions:         sessions,
		staticFS:         staticFS,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/dashboard", s.handleDashboard)
	mux.HandleFunc("/api/askpass", s.handleAskpassCreate)
	mux.HandleFunc("/api/askpass/", s.handleAskpassAction)
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/", s.handleFrontend)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func requestIDFromPath(path, prefix string) (string, bool) {
	id := strings.TrimPrefix(path, prefix)
	id = strings.Trim(id, "/")
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

func isJSONRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}
