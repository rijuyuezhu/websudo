package approverd

import "net/http"

type dashboardResponse struct {
	AskpassPending []AskpassRequest `json:"askpassPending"`
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
	s.expireAskpassRequests()

	writeJSON(w, http.StatusOK, dashboardResponse{
		AskpassPending: s.askpassStore.ListPending(),
	})
}
