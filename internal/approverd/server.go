package approverd

import (
	"encoding/json"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"strings"

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
	config            config.Config
	askpassStore      *AskpassStore
	verifyAskpassPeer func(net.Conn) (AskpassProvenance, error)
	passwordVerifier  PasswordVerifier
	sessions          *SessionStore
	staticFS          fs.FS
}

func NewServer(dep Dependencies) *Server {
	askpassStore := dep.AskpassStore
	if askpassStore == nil {
		askpassStore = NewAskpassStore()
	}
	askpassStore.setExpirationTimeout(dep.Config.ApprovalTimeout)
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
		config:       dep.Config,
		askpassStore: askpassStore,
		verifyAskpassPeer: func(conn net.Conn) (AskpassProvenance, error) {
			return verifyAskpassProcessChain(conn)
		},
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
	mux.HandleFunc("/api/askpass", http.NotFound)
	mux.HandleFunc("/api/askpass/", s.handleAskpassAction)
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/", s.handleFrontend)
	return browserSecurityHeaders(mux)
}

func browserSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
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
