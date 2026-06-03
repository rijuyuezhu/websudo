package approverd

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"time"

	"websudo/internal/config"
	"websudo/internal/model"
	"websudo/internal/rootd"
	storepkg "websudo/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

type Store interface {
	CreateRequest(req model.Request) error
	ExpirePendingRequests(before time.Time) (int, error)
	ListPendingRequests() ([]model.Request, error)
	ListRecentRequests() ([]model.Request, error)
	GetRequest(id string) (model.Request, error)
	ApproveRequest(id string) (model.Request, error)
	MarkRunning(id string) (model.Request, error)
	CompleteRequest(id string, result model.Result) (model.Request, error)
	DenyRequest(id string) (model.Request, error)
}

type Executor interface {
	Execute(context.Context, model.Command) (model.Result, error)
}

type Dependencies struct {
	Config           config.Config
	Store            Store
	AskpassStore     *AskpassStore
	Templates        *template.Template
	Executor         Executor
	PasswordVerifier PasswordVerifier
	SessionStore     *SessionStore
	StaticFS         fs.FS
}

type Server struct {
	config           config.Config
	store            Store
	askpassStore     *AskpassStore
	templates        *template.Template
	executor         Executor
	passwordVerifier PasswordVerifier
	sessions         *SessionStore
	staticFS         fs.FS
}

type RootExecutor struct {
	SocketPath string
}

type SQLiteStore struct {
	store *storepkg.SQLiteStore
}

func NewSQLiteStore(store *storepkg.SQLiteStore) *SQLiteStore {
	return &SQLiteStore{store: store}
}

func (s *SQLiteStore) ListPendingRequests() ([]model.Request, error) {
	return s.store.ListRequestsByStatus(context.Background(), model.StatusPending)
}

func (s *SQLiteStore) ExpirePendingRequests(before time.Time) (int, error) {
	return s.store.ExpirePendingRequests(context.Background(), before)
}

func (s *SQLiteStore) ListRecentRequests() ([]model.Request, error) {
	return s.store.ListRequestsExcludingStatus(context.Background(), model.StatusPending, 20)
}

func (s *SQLiteStore) GetRequest(id string) (model.Request, error) {
	return s.store.GetRequest(context.Background(), id)
}

func (s *SQLiteStore) CreateRequest(req model.Request) error {
	return s.store.CreateRequest(context.Background(), req)
}

func (s *SQLiteStore) ApproveRequest(id string) (model.Request, error) {
	return s.store.UpdateRequestStatus(context.Background(), id, model.StatusPending, model.StatusApproved)
}

func (s *SQLiteStore) MarkRunning(id string) (model.Request, error) {
	return s.store.UpdateRequestStatus(context.Background(), id, model.StatusApproved, model.StatusRunning)
}

func (s *SQLiteStore) CompleteRequest(id string, result model.Result) (model.Request, error) {
	return s.store.CompleteRequest(context.Background(), id, result)
}

func (s *SQLiteStore) DenyRequest(id string) (model.Request, error) {
	return s.store.UpdateRequestStatus(context.Background(), id, model.StatusPending, model.StatusDenied)
}

func NewServer(dep Dependencies) *Server {
	templates := dep.Templates
	if templates == nil {
		templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
	}
	executor := dep.Executor
	if executor == nil {
		executor = RootExecutor{SocketPath: dep.Config.RootSocketPath}
	}
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
		store:            dep.Store,
		askpassStore:     askpassStore,
		templates:        templates,
		executor:         executor,
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
	mux.HandleFunc("/api/browser/requests/", s.handleBrowserRequestDetail)
	mux.HandleFunc("/api/askpass", s.handleAskpassCreate)
	mux.HandleFunc("/api/askpass/", s.handleAskpassAction)
	mux.HandleFunc("/api/requests", s.handleRequests)
	mux.HandleFunc("/api/requests/", s.handleRequestAction)
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/", s.handleFrontend)
	return mux
}

func (e RootExecutor) Execute(ctx context.Context, command model.Command) (model.Result, error) {
	response, err := rootd.Execute(ctx, e.SocketPath, rootd.ExecRequest{
		ResolvedPath: command.ResolvedPath,
		Argv:         command.Argv,
		Cwd:          command.Cwd,
	})
	if err != nil {
		return model.Result{}, err
	}
	result := model.Result{
		ExitCode: response.Result.ExitCode,
		Signal:   response.Result.Signal,
		Stdout:   response.Result.Stdout,
		Stderr:   response.Result.Stderr,
	}
	if response.Error != "" {
		return result, errors.New(response.Error)
	}
	return result, nil
}

func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/requests" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req model.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	storedReq := model.NewRequest(req.ID(), req.CreatedAt(), req.RequestedBy(), req.Command())
	if err := s.store.CreateRequest(storedReq); err != nil {
		w.WriteHeader(http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, storedReq)
}

func (s *Server) handleRequestAction(w http.ResponseWriter, r *http.Request) {
	if err := s.expirePendingRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodGet {
		id, ok := requestIDFromPath(r.URL.Path, "/api/requests/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		req, err := s.store.GetRequest(id)
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

	id, action, ok := requestActionFromPath(r.URL.Path)
	if !ok {
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

	var err error
	switch action {
	case "approve":
		_, err = s.store.ApproveRequest(id)
		if err == nil {
			request, runErr := s.store.MarkRunning(id)
			if runErr != nil {
				err = runErr
				break
			}
			result, execErr := s.executor.Execute(r.Context(), request.Command())
			if execErr != nil {
				if result.ExitCode == 0 {
					result.ExitCode = 1
				}
				if result.Stderr == "" {
					result.Stderr = execErr.Error()
				}
			}
			_, err = s.store.CompleteRequest(id, result)
		}
	case "deny":
		_, err = s.store.DenyRequest(id)
	default:
		http.NotFound(w, r)
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusAccepted)
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

func requestActionFromPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/requests/"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func isJSONRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

func (s *Server) expirePendingRequests() error {
	if s.config.ApprovalTimeoutSeconds <= 0 || s.store == nil {
		return nil
	}
	_, err := s.store.ExpirePendingRequests(time.Now().Add(-time.Duration(s.config.ApprovalTimeoutSeconds) * time.Second).UTC())
	return err
}
