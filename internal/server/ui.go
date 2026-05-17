package server

import (
	"net/http"
	"strings"

	ingressIface "github.com/proxa-server/proxa/internal/ingress"
	"github.com/proxa-server/proxa/internal/web"
)

// uiData is the template payload for the dashboard pages. Wraps
// SystemStatus and adds derived counts the templates need.
type uiData struct {
	Node          NodeStatus
	Projects      []ProjectSummary
	Routes        []RouteRow
	TotalServices int
	TotalRoutes   int
	Ingress       IngressInfoRow
}

// RouteRow is one row in the dashboard's Routes card. TLSStatus is the
// string form of ingress.CertStatus ("valid", "renewing", etc.) so the
// template can switch on it directly.
type RouteRow struct {
	Project      string
	Host         string
	Path         string
	L4           string
	Port         int
	Service      string
	TLSStatus    string
	BackendCount int
}

// IngressInfoRow is the header widget summarizing the ingress.
type IngressInfoRow struct {
	HTTPPort   int
	HTTPSPort  int
	TLSEnabled bool
	CertCount  int
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
	s.Router.With(mw).Get("/ui/routes", s.handleUIRoutesFragment)

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

			// Routes: one row per [[route]] block. TLS state via
			// ingress.CertInfo when available; falls back to "off"
			// (also the right answer when ingress is nil).
			for _, route := range svc.Spec.Routes {
				row := RouteRow{
					Project:      p.Name,
					Host:         route.Host,
					Path:         route.Path,
					L4:           route.L4,
					Port:         route.Port,
					Service:      svc.Name,
					BackendCount: actual,
					TLSStatus:    string(ingressIface.CertStatusOff),
				}
				if s.ingress != nil && route.L4 == "" {
					if info, ok := s.ingress.CertInfo(route.Host); ok {
						row.TLSStatus = string(info.Status)
					}
				}
				out.Routes = append(out.Routes, row)
				out.TotalRoutes++
			}
		}
		out.Projects = append(out.Projects, ps)
	}

	// Ingress server-wide widget for the cluster-status header.
	if s.ingress != nil {
		info := s.ingress.IngressInfo()
		out.Ingress = IngressInfoRow{
			HTTPPort:   info.HTTPPort,
			HTTPSPort:  info.HTTPSPort,
			TLSEnabled: info.TLSEnabled,
			CertCount:  info.CertCount,
		}
	}
	return out
}
