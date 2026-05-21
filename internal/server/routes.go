package server

import "github.com/go-chi/chi/v5"

// MountRoutes registers the /api/v1 routes on the server's Router.
// Called by callers (e.g., proxa server cmd) after Server.New so the
// Router can be customized (e.g., for adding the slim dashboard's UI
// routes in Phase 7.5).
func (s *Server) MountRoutes() {
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(RequireAuth(s.authn))
		r.Get("/system/status", s.handleSystemStatus)
		r.Get("/system", s.handleSystemInfo)
		r.Get("/projects", s.handleListProjects)
		r.Post("/projects", s.handleCreateProject)
		r.Route("/projects/{project}/services", func(r chi.Router) {
			r.Get("/", s.handleListServices)
			r.Get("/{name}", s.handleGetService)
			r.Put("/{name}", s.handleUpsertService)
			r.Delete("/{name}", s.handleDeleteService)
			r.Post("/{name}/scale", s.handleScaleService)
			r.Get("/{name}/logs", s.handleStreamServiceLogs)
		})
		r.Get("/routes", s.handleListRoutes)
		r.Get("/ingress", s.handleGetIngress)
	})
}
