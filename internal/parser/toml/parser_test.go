package toml

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/proxa-server/proxa/pkg/types"
)

func TestParseValidMinimal(t *testing.T) {
	td := parseFile(t, "valid-minimal.toml")
	if td.Name != "web" {
		t.Errorf("name = %q, want 'web'", td.Name)
	}
	if td.Image != "nginx:alpine" {
		t.Errorf("image = %q, want 'nginx:alpine'", td.Image)
	}
	if td.Replicas != 3 {
		t.Errorf("replicas = %d, want 3", td.Replicas)
	}
	if td.Project != "default" {
		t.Errorf("project = %q, want 'default' (auto-applied)", td.Project)
	}
	if len(td.Expose) != 1 || td.Expose[0].Protocol != "http" {
		t.Errorf("expose mismatch: %+v", td.Expose)
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		fixture  string
		wantCode string
	}{
		{"invalid-missing-image.toml", "invalid-image-ref"},
		{"invalid-bad-name.toml", "missing-or-invalid-name"},
		{"invalid-bad-protocol.toml", "invalid-protocol"},
		{"invalid-health-mutual.toml", "health-mutually-exclusive"},
		{"invalid-health-needs-port.toml", "health-probe-needs-port"},
		{"invalid-health-timeout-too-big.toml", "health-timeout-out-of-range"},
		{"invalid-health-bad-retries.toml", "health-retries-out-of-range"},
		{"invalid-health-bad-via.toml", "health-bad-via"},
		{"invalid-route-needs-host.toml", "route-needs-host"},
		{"invalid-route-bad-host.toml", "route-bad-host"},
		{"invalid-route-bad-path.toml", "route-bad-path"},
		{"invalid-route-bad-l4.toml", "route-invalid-protocol"},
		{"invalid-route-needs-port.toml", "route-needs-port"},
		{"invalid-route-bad-lb.toml", "route-bad-lb"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			_, err := tryParse(t, tt.fixture)
			if err == nil {
				t.Fatalf("expected error for %s", tt.fixture)
			}
			var ve *validationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *validationError, got %T: %v", err, err)
			}
			if ve.Code != tt.wantCode {
				t.Errorf("code = %q, want %q (err: %v)", ve.Code, tt.wantCode, err)
			}
		})
	}
}

func TestParseValidRoutes(t *testing.T) {
	cases := []struct {
		fixture     string
		wantRoutes  int
		wantHost    string
		wantL4      string
	}{
		{"valid-route-tls.toml", 1, "whoami.example.com", ""},
		{"valid-route-l4-tcp.toml", 1, "cache.example.com", "tcp"},
		{"valid-route-multi.toml", 2, "api.example.com", ""},
	}
	for _, tt := range cases {
		t.Run(tt.fixture, func(t *testing.T) {
			td := parseFile(t, tt.fixture)
			if len(td.Routes) != tt.wantRoutes {
				t.Fatalf("routes=%d, want %d", len(td.Routes), tt.wantRoutes)
			}
			if td.Routes[0].Host != tt.wantHost {
				t.Errorf("routes[0].host=%q, want %q", td.Routes[0].Host, tt.wantHost)
			}
			if td.Routes[0].L4 != tt.wantL4 {
				t.Errorf("routes[0].l4=%q, want %q", td.Routes[0].L4, tt.wantL4)
			}
		})
	}
}

func parseFile(t *testing.T, name string) types.TaskDef {
	t.Helper()
	td, err := tryParse(t, name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return td
}

func tryParse(t *testing.T, name string) (types.TaskDef, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	return Parse(f)
}
