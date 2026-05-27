package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/proxa-server/proxa/internal/web"
)

// uiHostLogsData is the template payload for /ui/logs/host/{id}.
type uiHostLogsData struct {
	ContainerID string
}

// handleUIHostLogs renders the full-page host-container log viewer.
// Validates that the container exists (404 otherwise) so the page
// doesn't render against a typo'd id.
func (s *Server) handleUIHostLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	info, err := s.runtime.InspectContainer(r.Context(), id)
	if err != nil || info == nil {
		writeError(w, http.StatusNotFound, "container-not-found", "no such container "+id)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "host_logs.html", uiHostLogsData{ContainerID: id}); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}
