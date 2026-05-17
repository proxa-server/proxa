package toml

import (
	"context"
	"fmt"
	"regexp"

	"github.com/proxa-server/proxa/internal/security"
	"github.com/proxa-server/proxa/pkg/types"
)

var _ = types.TaskDef{} // keep types import explicit even if the per-func references vary

// Each error code below is stable and matches contracts/toml-grammar.md.
type validationError struct {
	Code    string
	Message string
}

func (e *validationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Code returns the stable error code, so the CLI can prefix user output.
func (e *validationError) ErrCode() string { return e.Code }

var (
	nameRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	cpuRe      = regexp.MustCompile(`^\d+m?$`)
	memoryRe   = regexp.MustCompile(`^\d+(Ki|Mi|Gi)?$`)
	// hostnameRe rejects leading hyphens, empty labels, trailing dots.
	// Wildcard ("*.example.com") is intentionally NOT accepted in v0.3.
	hostnameRe       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
	allowedProtocols = map[string]bool{"tcp": true, "udp": true, "http": true, "https": true}
	allowedStrategies = map[types.DeployStrategy]bool{
		types.StrategyStartFirst: true,
		types.StrategyStopFirst:  true,
		"":                      true, // optional; default applied later
	}
	allowedL4         = map[string]bool{"": true, "tcp": true, "udp": true}
	allowedLB         = map[string]bool{"": true, "random": true, "round-robin": true}
	allowedHealthVia  = map[string]bool{"": true, "direct": true, "ingress": true}
)

// Validate checks every rule from contracts/toml-grammar.md and returns
// the first violation with its stable error code.
func Validate(td types.TaskDef) error {
	if !nameRe.MatchString(td.Project) {
		return &validationError{Code: "invalid-project-name", Message: fmt.Sprintf("project %q does not match ^[a-z0-9][a-z0-9-]{0,62}$", td.Project)}
	}
	if !nameRe.MatchString(td.Name) {
		return &validationError{Code: "missing-or-invalid-name", Message: fmt.Sprintf("name %q is required and must match ^[a-z0-9][a-z0-9-]{0,62}$", td.Name)}
	}
	if td.Image == "" {
		return &validationError{Code: "invalid-image-ref", Message: "image is required"}
	}
	if td.Replicas < 0 {
		return &validationError{Code: "invalid-replicas", Message: fmt.Sprintf("replicas must be >= 0, got %d", td.Replicas)}
	}
	if !allowedStrategies[td.Strategy] {
		return &validationError{Code: "invalid-strategy", Message: fmt.Sprintf("strategy %q is not one of start-first|stop-first", td.Strategy)}
	}
	if err := security.Validate(td.Security); err != nil {
		// Map to the appropriate grammar code based on the error.
		msg := err.Error()
		if contains(msg, "user 'root' requires") {
			return &validationError{Code: "root-requires-allowroot", Message: msg}
		}
		if contains(msg, "noNewPrivileges=false requires") {
			return &validationError{Code: "nonewprivileges-requires-allowroot", Message: msg}
		}
		return &validationError{Code: "invalid-security", Message: msg}
	}
	for i, p := range td.Expose {
		if !allowedProtocols[p.Protocol] {
			return &validationError{Code: "invalid-protocol", Message: fmt.Sprintf("expose[%d].protocol %q must be tcp|udp|http|https", i, p.Protocol)}
		}
		if p.Container < 1 || p.Container > 65535 {
			return &validationError{Code: "invalid-port", Message: fmt.Sprintf("expose[%d].container %d out of 1..65535", i, p.Container)}
		}
		if p.Host < 0 || p.Host > 65535 {
			return &validationError{Code: "invalid-port", Message: fmt.Sprintf("expose[%d].host %d out of 0..65535", i, p.Host)}
		}
	}
	for i, v := range td.Volumes {
		if v.Source == "" {
			return &validationError{Code: "invalid-volume-source", Message: fmt.Sprintf("volumes[%d].source is required", i)}
		}
	}
	if td.Resources.CPU != "" && !cpuRe.MatchString(td.Resources.CPU) {
		return &validationError{Code: "invalid-resource", Message: fmt.Sprintf("resources.cpu %q must match ^\\d+m?$", td.Resources.CPU)}
	}
	if td.Resources.Memory != "" && !memoryRe.MatchString(td.Resources.Memory) {
		return &validationError{Code: "invalid-resource", Message: fmt.Sprintf("resources.memory %q must match ^\\d+(Ki|Mi|Gi)?$", td.Resources.Memory)}
	}
	if err := validateHealth(td); err != nil {
		return err
	}
	if err := validateRoutes(td); err != nil {
		return err
	}
	// schedule cron validation is shallow in v0.0 — accept any non-empty string;
	// full cron parser arrives with the scheduler in Feature 002.
	return nil
}

// validateRoutes enforces the [[route]] block rules per
// specs/003-ingress/contracts/route.md:
//   - L7 (l4 == "") requires non-empty host (route-needs-host)
//   - host must be a valid DNS name; wildcards rejected (route-bad-host)
//   - path must start with / and only the trailing * is allowed (route-bad-path)
//   - l4 ∈ {"", "tcp", "udp"} (route-invalid-protocol)
//   - L4 requires port 1..65535 (route-needs-port)
//   - lb_strategy ∈ {"", "random", "round-robin"} (route-bad-lb)
//
// Per-project / cross-project (host, path) collision is enforced at
// proxa-up time via the StateStore-aware ValidateAgainstStore helper
// (see T005); this function handles only the per-TOML rules.
func validateRoutes(td types.TaskDef) error {
	for i, r := range td.Routes {
		if !allowedL4[r.L4] {
			return &validationError{Code: "route-invalid-protocol", Message: fmt.Sprintf("route[%d].l4 %q must be tcp|udp or empty for L7", i, r.L4)}
		}
		if r.L4 == "" { // L7
			if r.Host == "" {
				return &validationError{Code: "route-needs-host", Message: fmt.Sprintf("route[%d] is L7 (l4 unset) and requires host", i)}
			}
		} else { // L4
			if r.Port < 1 || r.Port > 65535 {
				return &validationError{Code: "route-needs-port", Message: fmt.Sprintf("route[%d].port %d out of 1..65535 for l4=%s", i, r.Port, r.L4)}
			}
		}
		if r.Host != "" && !hostnameRe.MatchString(r.Host) {
			return &validationError{Code: "route-bad-host", Message: fmt.Sprintf("route[%d].host %q must be a valid DNS name (no wildcards in v0.3)", i, r.Host)}
		}
		if err := validateRoutePath(i, r.Path); err != nil {
			return err
		}
		if !allowedLB[r.LBStrategy] {
			return &validationError{Code: "route-bad-lb", Message: fmt.Sprintf("route[%d].lb_strategy %q must be random|round-robin or empty", i, r.LBStrategy)}
		}
	}
	return nil
}

func validateRoutePath(i int, p string) error {
	if p == "" {
		return nil
	}
	if p[0] != '/' {
		return &validationError{Code: "route-bad-path", Message: fmt.Sprintf("route[%d].path %q must start with /", i, p)}
	}
	// Only one trailing * is allowed; reject any other use of *.
	for j := 0; j < len(p); j++ {
		if p[j] != '*' {
			continue
		}
		if j != len(p)-1 {
			return &validationError{Code: "route-bad-path", Message: fmt.Sprintf("route[%d].path %q: * is only allowed as the last character", i, p)}
		}
	}
	return nil
}

// validateHealth enforces the [health] block rules per
// specs/002-health-checks/data-model.md:
//   - path and command are mutually exclusive
//   - if path set, a port must be resolvable (from health.port OR first expose)
//   - timeout must be > 0 and <= interval (when both present)
//   - retries must be 1..100 (when present; 0 means "use default")
func validateHealth(td types.TaskDef) error {
	h := td.Health
	hasPath := h.Path != ""
	hasCommand := len(h.Command) > 0

	if hasPath && hasCommand {
		return &validationError{
			Code:    "health-mutually-exclusive",
			Message: "health.path and health.command are mutually exclusive",
		}
	}
	if hasPath {
		port := h.Port
		if port == 0 && len(td.Expose) > 0 {
			port = td.Expose[0].Container
		}
		if port == 0 {
			return &validationError{
				Code:    "health-probe-needs-port",
				Message: "health.path set but no port resolvable (set health.port or declare at least one [[expose]])",
			}
		}
	}
	if h.Timeout > 0 && h.Interval > 0 && h.Timeout > h.Interval {
		return &validationError{
			Code:    "health-timeout-out-of-range",
			Message: fmt.Sprintf("health.timeout %s must be <= health.interval %s", h.Timeout, h.Interval),
		}
	}
	if h.Retries < 0 || h.Retries > 100 {
		return &validationError{
			Code:    "health-retries-out-of-range",
			Message: fmt.Sprintf("health.retries %d must be in 1..100 (or 0 for default)", h.Retries),
		}
	}
	if !allowedHealthVia[h.Via] {
		return &validationError{
			Code:    "health-bad-via",
			Message: fmt.Sprintf("health.via %q must be direct|ingress or empty", h.Via),
		}
	}
	return nil
}

// RouteLookup is the minimal store surface ValidateAgainstStore needs.
// Implemented by internal/store.StateStore — declared here as a tiny
// interface so the parser package keeps its narrow import surface.
type RouteLookup interface {
	ListProjects(ctx context.Context) ([]types.Project, error)
	ListServices(ctx context.Context, project string) ([]types.Service, error)
}

// ValidateAgainstStore runs Validate AND a project-scoped + cross-project
// route-conflict check (§III + FR-017) that compares td.Routes against
// every other service already in the store. Replacing a service's own
// routes is always allowed (skipped from the comparison).
func ValidateAgainstStore(ctx context.Context, td types.TaskDef, lookup RouteLookup) error {
	if err := Validate(td); err != nil {
		return err
	}
	if len(td.Routes) == 0 || lookup == nil {
		return nil
	}
	projects, err := lookup.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("parser/toml: list projects for route check: %w", err)
	}
	for _, p := range projects {
		services, err := lookup.ListServices(ctx, p.Name)
		if err != nil {
			return fmt.Errorf("parser/toml: list services for route check: %w", err)
		}
		for _, svc := range services {
			if svc.Project == td.Project && svc.Name == td.Name {
				continue // same service being updated
			}
			for _, existing := range svc.Spec.Routes {
				for _, newR := range td.Routes {
					if routesConflict(newR, existing, td.Project, svc.Project) {
						return &validationError{
							Code: "route-conflict",
							Message: fmt.Sprintf(
								"route %q conflicts with project %q service %q route %q",
								routeStr(newR), svc.Project, svc.Name, routeStr(existing)),
						}
					}
				}
			}
		}
	}
	return nil
}

func routesConflict(a, b types.Route, aProject, bProject string) bool {
	if a.Host != b.Host {
		return false
	}
	// Cross-project: any host collision is a conflict (FR-017 + §III).
	if aProject != bProject {
		return true
	}
	// Same project: only (host, path) collision is a conflict.
	return a.Path == b.Path
}

func routeStr(r types.Route) string {
	if r.L4 != "" {
		return fmt.Sprintf("%s://%s:%d", r.L4, r.Host, r.Port)
	}
	return r.Host + r.Path
}

// contains is a tiny strings.Contains alternative.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
