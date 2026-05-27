package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/proxa-server/proxa/internal/events"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
)

// hostContainer is the JSON shape returned by /api/v1/host-containers.
// Distinct from runtime.ContainerInfo so the API contract can evolve
// without dragging the runtime interface with it.
type hostContainer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	State   string            `json:"state"`
	Managed bool              `json:"managed"`
	Project string            `json:"project,omitempty"`
	Service string            `json:"service,omitempty"`
	Replica string            `json:"replica,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// handleListHostContainers responds to GET /api/v1/host-containers.
// Lists every container Docker can see, annotating each with a
// `managed` boolean derived from the proxa.project label.
func (s *Server) handleListHostContainers(w http.ResponseWriter, r *http.Request) {
	cs, err := s.runtime.ListAllContainers(r.Context())
	if err != nil {
		http.Error(w, "host-containers: list failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]hostContainer, 0, len(cs))
	for _, c := range cs {
		out = append(out, toHostContainer(c))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"containers": out})
}

// handleHostContainerAction dispatches start/stop/restart actions.
// Refuses (409) on managed containers; emits the matching user.container.*
// event on success.
func (s *Server) handleHostContainerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		info, err := s.runtime.InspectContainer(r.Context(), id)
		if err != nil || info == nil {
			http.Error(w, "host-container not found", http.StatusNotFound)
			return
		}
		if managed, _ := managedFromLabels(info.Labels); managed {
			http.Error(w, "container is Proxa-managed; use `proxa scale` or update the TOML", http.StatusConflict)
			return
		}

		prevState := info.State
		var actErr error
		var eventType string
		switch action {
		case "start":
			actErr = s.runtime.StartContainer(r.Context(), id)
			eventType = events.TypeUserContainerStart
		case "stop":
			actErr = s.runtime.StopContainer(r.Context(), id, 0)
			eventType = events.TypeUserContainerStop
		case "restart":
			actErr = s.runtime.RestartContainer(r.Context(), id, 0)
			eventType = events.TypeUserContainerRestart
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		if actErr != nil {
			http.Error(w, "host-container "+action+": "+actErr.Error(), http.StatusInternalServerError)
			return
		}
		s.emitHostContainerEvent(r, eventType, id, info.Name, prevState)
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleHostContainerRemove DELETE /api/v1/host-containers/{id}.
// Refuses managed (409). Default force=false; ?force=true allows
// removing a running container.
func (s *Server) handleHostContainerRemove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	info, err := s.runtime.InspectContainer(r.Context(), id)
	if err != nil || info == nil {
		http.Error(w, "host-container not found", http.StatusNotFound)
		return
	}
	if managed, _ := managedFromLabels(info.Labels); managed {
		http.Error(w, "container is Proxa-managed; use `proxa scale` or update the TOML", http.StatusConflict)
		return
	}
	force := false
	if raw := r.URL.Query().Get("force"); raw != "" {
		b, _ := strconv.ParseBool(raw)
		force = b
	}
	if info.State == "running" && !force {
		http.Error(w, "container is running; stop it first or pass ?force=true", http.StatusConflict)
		return
	}
	if err := s.runtime.RemoveContainer(r.Context(), id, force); err != nil {
		http.Error(w, "host-container remove: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.emitHostContainerEvent(r, events.TypeUserContainerRemove, id, info.Name, info.State)
	w.WriteHeader(http.StatusNoContent)
}

// emitHostContainerEvent records one host-container action in the
// events table. Best-effort: failures are logged via the server's
// audit-store error path, never propagated to the HTTP response (the
// action itself already succeeded).
//
// Actor is hard-coded to "subject:bootstrap-admin" in v0.4.4 — replaced
// with the authenticated subject id when v0.5 multi-user lands.
func (s *Server) emitHostContainerEvent(r *http.Request, typ, id, name, prevState string) {
	if s.events == nil {
		return
	}
	payload := fmt.Sprintf(`{"name":%q,"prev_state":%q}`, name, prevState)
	_, _ = s.events.Append(r.Context(), events.Event{
		Type:    typ,
		Actor:   "subject:bootstrap-admin",
		Target:  events.TargetContainer(shortID(id)),
		Payload: payload,
	})
}

// shortID returns the first 12 hex chars of a Docker ID, matching
// `docker ps` and the dashboard's display convention.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// managedFromLabels reports whether a container is Proxa-managed and
// (if so) what project owns it.
func managedFromLabels(labels map[string]string) (bool, string) {
	if labels == nil {
		return false, ""
	}
	if p, ok := labels[dockerlabels.LabelProject]; ok && p != "" {
		return true, p
	}
	return false, ""
}

func toHostContainer(c rt.ContainerInfo) hostContainer {
	managed, proj := managedFromLabels(c.Labels)
	hc := hostContainer{
		ID:      shortID(c.ID),
		Name:    c.Name,
		Image:   c.Image,
		State:   c.State,
		Managed: managed,
		Project: proj,
		Labels:  c.Labels,
	}
	if managed {
		hc.Service = c.Labels[dockerlabels.LabelService]
		hc.Replica = c.Labels[dockerlabels.LabelReplica]
	}
	return hc
}
