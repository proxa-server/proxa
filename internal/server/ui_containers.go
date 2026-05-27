package server

import (
	"net/http"

	"github.com/proxa-server/proxa/internal/web"
)

// handleUIContainers renders the full-page host containers viewer
// (/ui/containers). Pure Alpine.js — polls /api/v1/host-containers for
// data. Action buttons POST/DELETE the same API; this handler has no
// state.
func (s *Server) handleUIContainers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "containers.html", nil); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}
