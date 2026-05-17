package server

import (
	"net/http"

	"github.com/proxa-server/proxa/internal/web"
)

// handleUIRoutesFragment serves the HTMX poll target for the Routes
// card. Same shape as handleUIServicesFragment.
func (s *Server) handleUIRoutesFragment(w http.ResponseWriter, r *http.Request) {
	data := s.buildUIData(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "routes_table.html", data); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}
