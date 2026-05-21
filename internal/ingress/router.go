package ingress

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/proxa-server/proxa/pkg/types"
)

// Router holds the immutable routing snapshot. Built once per reconciler
// tick by BuildRouter and swapped into RouterPtr via Swap.
//
// L7 lookups walk the l7Routes slice (sorted longest-prefix-first); for
// typical N ≤ 100 routes this is faster than a trie due to cache
// locality. L4 lookups are a map (proto, port) → service.
type Router struct {
	l7Routes []l7Route
	l4Routes map[l4Key]l4Route
}

type l7Route struct {
	Project  string
	Host     string
	PathGlob string // "" (catch-all), "/v1/users" (exact), "/v1/*" (prefix)
	Service  ServiceID
	LB       string
}

type l4Route struct {
	Project string
	Service ServiceID
	LB      string
}

type l4Key struct {
	Proto string
	Port  int
}

// LookupL7 resolves an HTTP request to a service. Returns (svc, lbStrategy, true)
// on a match. Path matching: longest matching glob wins; "/v1/*" matches
// any path starting with "/v1"; exact paths match exactly.
func (r *Router) LookupL7(host, path string) (ServiceID, string, bool) {
	if r == nil {
		return ServiceID{}, "", false
	}
	for _, rt := range r.l7Routes {
		if rt.Host != host {
			continue
		}
		if matchPath(rt.PathGlob, path) {
			return rt.Service, rt.LB, true
		}
	}
	return ServiceID{}, "", false
}

// LookupL4 resolves an L4 listener (proto, port) to a service.
func (r *Router) LookupL4(proto string, port int) (ServiceID, string, bool) {
	if r == nil {
		return ServiceID{}, "", false
	}
	rt, ok := r.l4Routes[l4Key{Proto: proto, Port: port}]
	if !ok {
		return ServiceID{}, "", false
	}
	return rt.Service, rt.LB, true
}

// matchPath returns true when path satisfies glob's semantics.
//
//	glob == ""         → catch-all (matches any path for the hostname)
//	glob == "/v1/*"    → prefix match (everything under /v1, including /v1)
//	glob == "/v1/users" → exact match
func matchPath(glob, path string) bool {
	if glob == "" {
		return true
	}
	if before, ok := strings.CutSuffix(glob, "/*"); ok {
		prefix := before
		return path == prefix || strings.HasPrefix(path, prefix+"/") || path == prefix+"/"
	}
	if before, ok := strings.CutSuffix(glob, "*"); ok {
		// trailing * not preceded by / — treat as raw prefix.
		return strings.HasPrefix(path, before)
	}
	return glob == path
}

// BuildRouter constructs a Router snapshot from the project-scoped route
// map produced by the reconciler each tick. Returns route-conflict on
// any duplicate (host, path) within a project OR any cross-project host
// collision; this is the runtime-side guard against state that slipped
// past the parser's ValidateAgainstStore.
func BuildRouter(routes map[ServiceID][]types.Route) (*Router, error) {
	r := &Router{
		l4Routes: make(map[l4Key]l4Route),
	}

	// Track every (host, path) for in-project conflicts and every host
	// for cross-project conflicts.
	type hpKey struct{ project, host, path string }
	hostToProject := map[string]string{}
	seenHP := map[hpKey]bool{}

	for svcID, svcRoutes := range routes {
		for _, rt := range svcRoutes {
			if rt.L4 != "" {
				key := l4Key{Proto: rt.L4, Port: rt.Port}
				if existing, ok := r.l4Routes[key]; ok {
					return nil, fmt.Errorf("route-conflict: %s:%d already routed to %s/%s",
						key.Proto, key.Port, existing.Project, existing.Service.Service)
				}
				r.l4Routes[key] = l4Route{Project: svcID.Project, Service: svcID, LB: rt.LBStrategy}
				continue
			}

			// L7 cross-project host check
			if owner, ok := hostToProject[rt.Host]; ok && owner != svcID.Project {
				return nil, fmt.Errorf("route-conflict: host %q already routed in project %q (cross-project hosts not allowed)",
					rt.Host, owner)
			}
			hostToProject[rt.Host] = svcID.Project

			// In-project (host, path) conflict
			key := hpKey{project: svcID.Project, host: rt.Host, path: rt.Path}
			if seenHP[key] {
				return nil, fmt.Errorf("route-conflict: (%s, %s) declared twice in project %q",
					rt.Host, rt.Path, svcID.Project)
			}
			seenHP[key] = true

			r.l7Routes = append(r.l7Routes, l7Route{
				Project:  svcID.Project,
				Host:     rt.Host,
				PathGlob: rt.Path,
				Service:  svcID,
				LB:       rt.LBStrategy,
			})
		}
	}

	// Sort longest-prefix first so LookupL7 picks the most specific match.
	// Exact paths (no trailing *) outrank wildcard paths of the same length.
	slices.SortFunc(r.l7Routes, func(a, b l7Route) int {
		la, lb := len(a.PathGlob), len(b.PathGlob)
		if la != lb {
			return cmp.Compare(lb, la) // longest first
		}
		// Exact > wildcard at equal length.
		aw := strings.HasSuffix(a.PathGlob, "*")
		bw := strings.HasSuffix(b.PathGlob, "*")
		if aw != bw {
			if !aw {
				return -1 // exact (a) before wildcard (b)
			}
			return 1
		}
		// Deterministic tiebreaker by host then path.
		if c := cmp.Compare(a.Host, b.Host); c != 0 {
			return c
		}
		return cmp.Compare(a.PathGlob, b.PathGlob)
	})

	return r, nil
}
