package approverd

import (
	"net/http"

	"websudo/internal/model"
)

type dashboardResponse struct {
	AskpassPending []AskpassRequest `json:"askpassPending"`
	Pending        []model.Request  `json:"pending"`
	Recent         []model.Request  `json:"recent"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/dashboard" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireSession(w, r) {
		return
	}
	if err := s.expirePendingRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.expireAskpassRequests()

	pending := []model.Request{}
	recent := []model.Request{}
	if s.store != nil {
		var err error
		pending, err = s.store.ListPendingRequests()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		recent, err = s.store.ListRecentRequests()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if pending == nil {
		pending = []model.Request{}
	}
	if recent == nil {
		recent = []model.Request{}
	}
	writeJSON(w, http.StatusOK, dashboardResponse{
		AskpassPending: s.askpassStore.ListPending(),
		Pending:        pending,
		Recent:         recent,
	})
}

func (s *Server) handleBrowserRequestDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.requireSession(w, r) {
		return
	}
	if err := s.expirePendingRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.store == nil {
		http.NotFound(w, r)
		return
	}
	id, ok := requestIDFromPath(r.URL.Path, "/api/browser/requests/")
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
}
