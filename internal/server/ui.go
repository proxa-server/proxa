package server

import (
	"net/http"
	"strings"

	"github.com/proxa-server/proxa/internal/web"
)

// uiData is the template payload for the dashboard pages. Wraps
// SystemStatus and adds derived counts the templates need.
type uiData struct {
	Node          NodeStatus
	Projects      []ProjectSummary
	TotalServices int
}

// MountUI registers the slim dashboard routes on the server's Router.
// Per FR-017: auth bypass on Unix-socket connections; TCP requires
// the same Bearer token as /api/v1.
//
// In v0.0, /ui/ over TCP without a token returns 401 (the dashboard's
// login form arrives in Feature 004). Most operators use Proxa over
// the Unix socket where the bypass applies.
func (s *Server) MountUI() {
	// Bypass-or-require middleware honors FR-017.
	mw := RequireAuthOrUnix(s.authn, s.IsUnixListener())

	s.Router.With(mw).Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	})
	s.Router.With(mw).Get("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	})
	s.Router.With(mw).Get("/ui/", s.handleUIIndex)
	s.Router.With(mw).Get("/ui/services", s.handleUIServicesFragment)

	// Static assets — also gated by the same middleware so a TCP listener
	// without auth doesn't leak the JS/CSS (low-risk but consistent).
	s.Router.With(mw).Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(web.StaticFS))))
}

func (s *Server) handleUIIndex(w http.ResponseWriter, r *http.Request) {
	// If the user landed via /ui/?token=... promote that into a cookie
	// so subsequent HTMX polls include it automatically. Then redirect
	// to a clean URL so the token doesn't linger in the URL bar.
	if tok := r.URL.Query().Get("token"); tok != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "proxa_token",
			Value:    tok,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
		return
	}
	data := s.buildUIData(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "index.html", data); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}

func (s *Server) handleUIServicesFragment(w http.ResponseWriter, r *http.Request) {
	data := s.buildUIData(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(w, "services_table.html", data); err != nil {
		writeError(w, http.StatusInternalServerError, "render-failed", err.Error())
	}
}

// buildUIData fetches the system status (same aggregator the JSON
// /api/v1/system/status endpoint uses) and shapes it for the templates.
func (s *Server) buildUIData(r *http.Request) uiData {
	ctx := r.Context()
	out := uiData{Node: NodeStatus{ID: "node-local", Status: "ready"}}

	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return out
	}

	for _, p := range projects {
		ps := ProjectSummary{Name: p.Name}
		svcs, err := s.store.ListServices(ctx, p.Name)
		if err != nil {
			continue
		}
		actualByService := map[string]int{}
		if containers, err := s.runtime.ListContainers(ctx, makeListFilter(p.Name)); err == nil {
			for _, c := range containers {
				if strings.EqualFold(c.State, "running") {
					actualByService[c.Labels["proxa.service"]]++
				}
			}
			out.Node.ContainerCount += len(containers)
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
			out.TotalServices++
		}
		out.Projects = append(out.Projects, ps)
	}
	return out
}
