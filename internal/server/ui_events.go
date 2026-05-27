package server

import (
	"net/http"

	"github.com/proxa-server/proxa/internal/web"
)

// handleUIEvents renders the full-page events viewer (/ui/events).
// The page is a thin Alpine.js shell — it polls /api/v1/events for data
// rather than server-side-rendering rows, so the page itself doesn't
// need access to the events.Store. The API endpoint handles the 503
// case when no store is wired.
func (s *Server) handleUIEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "events.html", nil); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}
