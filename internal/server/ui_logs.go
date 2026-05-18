package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/internal/web"
)

// uiLogsData is the template payload for /ui/logs/{project}/{service}.
type uiLogsData struct {
	Project  string
	Service  string
	Replicas []int // 0..N-1 to populate the dropdown
}

// handleUILogs renders the full-page log viewer (templates/logs.html).
// 404s when the service has zero containers in the named project.
func (s *Server) handleUILogs(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	service := chi.URLParam(r, "service")

	// Resolve replica count so the dropdown shows the right options.
	containers, err := s.runtime.ListContainers(r.Context(), rt.ListFilter{Project: project})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "runtime-error", err.Error())
		return
	}
	count := 0
	for _, c := range containers {
		if c.Labels[dockerlabels.LabelService] == service {
			count++
		}
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "service-not-found",
			"no running containers for service "+service+" in project "+project)
		return
	}

	replicas := make([]int, 0, count)
	for i := 0; i < count; i++ {
		replicas = append(replicas, i)
	}

	data := uiLogsData{
		Project:  project,
		Service:  service,
		Replicas: replicas,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "logs.html", data); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}
