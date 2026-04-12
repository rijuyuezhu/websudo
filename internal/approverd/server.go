package approverd

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
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
	Config    config.Config
	Store     Store
	Templates *template.Template
	Executor  Executor
}

type Server struct {
	config    config.Config
	store     Store
	templates *template.Template
	executor  Executor
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

	return &Server{
		config:    dep.Config,
		store:     dep.Store,
		templates: templates,
		executor:  executor,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/requests/", s.handleRequestPage)
	mux.HandleFunc("/api/requests", s.handleRequests)
	mux.HandleFunc("/api/requests/", s.handleRequestAction)
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

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.store == nil {
		http.Error(w, "store not configured", http.StatusInternalServerError)
		return
	}
	if err := s.expirePendingRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pending, err := s.store.ListPendingRequests()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recent, err := s.store.ListRecentRequests()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "index.html", map[string]any{
		"Pending": pending,
		"Recent":  recent,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleRequestPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id, ok := requestIDFromPath(r.URL.Path, "/requests/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.expirePendingRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req, err := s.store.GetRequest(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "request.html", req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

	token, err := approvalToken(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !config.VerifyToken(s.config.TokenHashHex, token) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

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

	if isJSONRequest(r) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
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

func approvalToken(r *http.Request) (string, error) {
	if isJSONRequest(r) {
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return "", err
		}
		return body.Token, nil
	}

	if err := r.ParseForm(); err != nil {
		return "", err
	}
	if token := r.Form.Get("token"); token != "" {
		return token, nil
	}
	return "", errors.New("missing token")
}

func isJSONRequest(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}

func (s *Server) expirePendingRequests() error {
	if s.config.ApprovalTimeoutSeconds <= 0 || s.store == nil {
		return nil
	}
	_, err := s.store.ExpirePendingRequests(time.Now().Add(-time.Duration(s.config.ApprovalTimeoutSeconds) * time.Second).UTC())
	return err
}
