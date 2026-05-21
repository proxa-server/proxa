package server

import (
	"net/http"

	"github.com/proxa-server/proxa/internal/web"
)

// handleUISystem renders the focused System Info page (templates/system.html).
// The page fetches its data live from /api/v1/system on load + every 30s.
func (s *Server) handleUISystem(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "system.html", nil); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}
