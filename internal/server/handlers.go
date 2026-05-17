package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	parsertoml "github.com/proxa-server/proxa/internal/parser/toml"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// errCoder is implemented by parser validation errors that carry a
// stable error code.
type errCoder interface {
	ErrCode() string
}

// errCodeOf extracts a stable error code from a parser/validation error,
// falling back to "invalid-spec" for non-coded errors.
func errCodeOf(err error) string {
	var coder errCoder
	if errors.As(err, &coder) {
		return coder.ErrCode()
	}
	return "invalid-spec"
}

// SystemStatus is the response shape for GET /api/v1/system/status.
type SystemStatus struct {
	Node     NodeStatus       `json:"node"`
	Projects []ProjectSummary `json:"projects"`
}

type NodeStatus struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	ContainerCount int    `json:"containerCount"`
}

type ProjectSummary struct {
	Name     string           `json:"name"`
	Services []ServiceSummary `json:"services"`
}

type ServiceSummary struct {
	Name            string `json:"name"`
	Image           string `json:"image"`
	DesiredReplicas int    `json:"desiredReplicas"`
	ActualReplicas  int    `json:"actualReplicas"`
	Status          string `json:"status"`
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}

	resp := SystemStatus{
		Node: NodeStatus{ID: "node-local", Status: "ready"},
	}
	for _, p := range projects {
		ps := ProjectSummary{Name: p.Name}
		svcs, err := s.store.ListServices(ctx, p.Name)
		if err != nil {
			continue
		}
		// Best-effort actual count via Runtime.
		actualByService := map[string]int{}
		if containers, err := s.runtime.ListContainers(ctx, makeListFilter(p.Name)); err == nil {
			for _, c := range containers {
				if c.State == "running" {
					actualByService[c.Labels["proxa.service"]]++
				}
			}
			resp.Node.ContainerCount += len(containers)
		}
		for _, svc := range svcs {
			actual := actualByService[svc.Name]
			ps.Services = append(ps.Services, ServiceSummary{
				Name:            svc.Name,
				Image:           svc.Spec.Image,
				DesiredReplicas: svc.Spec.Replicas,
				ActualReplicas:  actual,
				Status:          serviceStatus(svc, actual),
			})
		}
		resp.Projects = append(resp.Projects, ps)
	}
	writeJSON(w, http.StatusOK, resp)
}

// serviceStatus prefers the persisted Service.Status (written by the
// reconciler from probe Snapshots) and falls back to a count-derived
// status when the reconciler hasn't populated it yet (first tick / pre-002
// services). Keeping the fallback means the dashboard stays useful even
// when probes are disabled.
func serviceStatus(svc types.Service, actual int) string {
	if svc.Status != "" {
		return string(svc.Status)
	}
	return deriveStatus(svc.Spec.Replicas, actual)
}

// deriveStatus computes a service's display status from desired vs actual
// running replica counts. Used only when persisted Status is empty.
func deriveStatus(desired, actual int) string {
	switch {
	case desired == 0 && actual == 0:
		return "stopped"
	case actual == desired:
		return "healthy"
	case actual < desired:
		return "reconciling"
	default: // actual > desired
		return "scaling-down"
	}
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p types.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-body", err.Error())
		return
	}
	p.CreatedAt = time.Now().UTC()
	if err := s.store.CreateProject(r.Context(), p); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already-exists", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	svcs, err := s.store.ListServices(r.Context(), project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, svcs)
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	svc, err := s.store.GetService(r.Context(), project, name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not-found", "service not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) handleUpsertService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	var spec types.TaskDef
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-body", err.Error())
		return
	}
	if spec.Project != project || spec.Name != name {
		writeError(w, http.StatusBadRequest, "spec-mismatch",
			"path project/name must match body project/name")
		return
	}

	// Ensure project exists; auto-create "default" but require explicit
	// creation for other projects.
	if _, err := s.store.GetProject(r.Context(), project); err != nil {
		if errors.Is(err, store.ErrNotFound) && project == "default" {
			_ = s.store.CreateProject(r.Context(), types.Project{Name: "default", CreatedAt: time.Now().UTC()})
		} else if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "project-not-found",
				"create the project before upserting a service in it")
			return
		}
	}

	// Cross-service route-conflict check (FR-017, §III). The CLI's
	// per-TOML Validate runs at parse time; this is the authoritative
	// check that sees every other service in every project.
	if err := parsertoml.ValidateAgainstStore(r.Context(), spec, s.store); err != nil {
		writeError(w, http.StatusBadRequest, errCodeOf(err), err.Error())
		return
	}

	svc := types.Service{
		Project: project,
		Name:    name,
		Spec:    spec,
	}
	if err := s.store.PutService(r.Context(), project, svc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-spec", err.Error())
		return
	}
	// Poke the reconciler so the operator sees fast convergence.
	if s.recon != nil {
		s.recon.Poke()
	}
	out, _ := s.store.GetService(r.Context(), project, name)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	if err := s.store.DeleteService(r.Context(), project, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not-found", "service not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}
	if s.recon != nil {
		s.recon.Poke()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScaleService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	var body struct{ Replicas int `json:"replicas"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-body", err.Error())
		return
	}
	if body.Replicas < 0 {
		writeError(w, http.StatusBadRequest, "invalid", "replicas must be >= 0")
		return
	}
	svc, err := s.store.GetService(r.Context(), project, name)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not-found", "service not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}
	svc.Spec.Replicas = body.Replicas
	if err := s.store.PutService(r.Context(), project, *svc); err != nil {
		writeError(w, http.StatusInternalServerError, "store-error", err.Error())
		return
	}
	if s.recon != nil {
		s.recon.Poke()
	}
	out, _ := s.store.GetService(r.Context(), project, name)
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// makeListFilter wraps the project name into a runtime ListFilter.
func makeListFilter(project string) (lf rtListFilter) {
	lf.Project = project
	return lf
}

// rtListFilter is a tiny alias to keep imports trim.
type rtListFilter = struct {
	Project string
	Labels  map[string]string
}
