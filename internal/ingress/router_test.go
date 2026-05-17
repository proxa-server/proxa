package ingress

import (
	"strings"
	"testing"

	"github.com/proxa-server/proxa/pkg/types"
)

func mkRoutes(routes ...types.Route) map[ServiceID][]types.Route {
	return map[ServiceID][]types.Route{
		{Project: "default", Service: "web"}: routes,
	}
}

func TestRouterLookupL7Catchall(t *testing.T) {
	r, err := BuildRouter(mkRoutes(types.Route{Host: "example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	svc, _, ok := r.LookupL7("example.com", "/anything")
	if !ok || svc.Service != "web" {
		t.Errorf("catchall lookup failed: ok=%v svc=%+v", ok, svc)
	}
}

func TestRouterLookupL7PrefixWildcard(t *testing.T) {
	r, err := BuildRouter(mkRoutes(types.Route{Host: "api.example.com", Path: "/v1/*"}))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		want bool
	}{
		{"/v1", true},
		{"/v1/", true},
		{"/v1/users", true},
		{"/v1/users/123", true},
		{"/v2/users", false},
		{"/", false},
	}
	for _, tc := range cases {
		_, _, ok := r.LookupL7("api.example.com", tc.path)
		if ok != tc.want {
			t.Errorf("LookupL7(/, %s)=%v, want %v", tc.path, ok, tc.want)
		}
	}
}

func TestRouterLookupL7ExactPath(t *testing.T) {
	r, err := BuildRouter(mkRoutes(types.Route{Host: "api.example.com", Path: "/v1/health"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := r.LookupL7("api.example.com", "/v1/health"); !ok {
		t.Errorf("exact path lookup failed")
	}
	if _, _, ok := r.LookupL7("api.example.com", "/v1/health/foo"); ok {
		t.Errorf("exact path should NOT match /v1/health/foo")
	}
}

func TestRouterLookupL7HostMismatch(t *testing.T) {
	r, err := BuildRouter(mkRoutes(types.Route{Host: "example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := r.LookupL7("other.com", "/"); ok {
		t.Errorf("hostname mismatch should miss")
	}
}

func TestRouterLookupL7LongestPrefixWins(t *testing.T) {
	routes := map[ServiceID][]types.Route{
		{Project: "default", Service: "web"}: {{Host: "api.example.com", Path: "/v1/*"}},
		{Project: "default", Service: "api"}: {{Host: "api.example.com", Path: "/v1/users/*"}},
	}
	r, err := BuildRouter(routes)
	if err != nil {
		t.Fatal(err)
	}
	svc, _, ok := r.LookupL7("api.example.com", "/v1/users/123")
	if !ok || svc.Service != "api" {
		t.Errorf("longest prefix should pick api, got svc=%+v ok=%v", svc, ok)
	}
}

func TestRouterLookupL4(t *testing.T) {
	r, err := BuildRouter(mkRoutes(types.Route{Host: "db.example.com", L4: "tcp", Port: 5432}))
	if err != nil {
		t.Fatal(err)
	}
	svc, _, ok := r.LookupL4("tcp", 5432)
	if !ok || svc.Service != "web" {
		t.Errorf("L4 lookup failed: ok=%v svc=%+v", ok, svc)
	}
	if _, _, ok := r.LookupL4("tcp", 5433); ok {
		t.Errorf("L4 wrong port should miss")
	}
	if _, _, ok := r.LookupL4("udp", 5432); ok {
		t.Errorf("L4 wrong proto should miss")
	}
}

func TestRouterBuildRejectsInProjectHostPathConflict(t *testing.T) {
	routes := map[ServiceID][]types.Route{
		{Project: "default", Service: "a"}: {{Host: "x.example.com"}},
		{Project: "default", Service: "b"}: {{Host: "x.example.com"}},
	}
	if _, err := BuildRouter(routes); err == nil || !strings.Contains(err.Error(), "route-conflict") {
		t.Errorf("expected route-conflict, got %v", err)
	}
}

func TestRouterBuildRejectsCrossProjectHostCollision(t *testing.T) {
	routes := map[ServiceID][]types.Route{
		{Project: "socio-do", Service: "web"}: {{Host: "shared.example.com"}},
		{Project: "kut-do", Service: "web"}:   {{Host: "shared.example.com", Path: "/v1/*"}},
	}
	if _, err := BuildRouter(routes); err == nil || !strings.Contains(err.Error(), "route-conflict") {
		t.Errorf("expected route-conflict on cross-project, got %v", err)
	}
}

func TestRouterBuildRejectsL4PortCollision(t *testing.T) {
	routes := map[ServiceID][]types.Route{
		{Project: "default", Service: "a"}: {{L4: "tcp", Port: 6379, Host: "a"}},
		{Project: "default", Service: "b"}: {{L4: "tcp", Port: 6379, Host: "b"}},
	}
	if _, err := BuildRouter(routes); err == nil || !strings.Contains(err.Error(), "route-conflict") {
		t.Errorf("expected route-conflict on L4 port, got %v", err)
	}
}

func TestRouterNilSafeLookup(t *testing.T) {
	var r *Router
	if _, _, ok := r.LookupL7("x", "/"); ok {
		t.Errorf("nil router L7 lookup should miss")
	}
	if _, _, ok := r.LookupL4("tcp", 80); ok {
		t.Errorf("nil router L4 lookup should miss")
	}
}
