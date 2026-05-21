package docker

import (
	"slices"
	"testing"

	"github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/security"
)

func TestApplySecurityProfile(t *testing.T) {
	t.Run("zero profile gets safe defaults including FR-002 user", func(t *testing.T) {
		cfg, host := applySecurityProfile(runtime.ContainerSpec{Image: "nginx:alpine"})

		if cfg.User != "1000:1000" {
			t.Errorf("FR-002: empty User without AllowRoot should default to '1000:1000', got %q", cfg.User)
		}
		if len(host.CapDrop) != 1 || host.CapDrop[0] != "ALL" {
			t.Errorf("CapDrop = %v, want [ALL]", host.CapDrop)
		}
		if !containsString(host.SecurityOpt, "no-new-privileges:true") {
			t.Errorf("SecurityOpt missing no-new-privileges: %v", host.SecurityOpt)
		}
	})

	t.Run("explicit User propagates verbatim", func(t *testing.T) {
		cfg, _ := applySecurityProfile(runtime.ContainerSpec{
			Image:    "nginx:alpine",
			Security: types_SecurityProfile{User: "500:500"},
		})
		if cfg.User != "500:500" {
			t.Errorf("explicit user not honored: %q", cfg.User)
		}
	})

	t.Run("AllowRoot leaves User empty so image USER applies", func(t *testing.T) {
		cfg, _ := applySecurityProfile(runtime.ContainerSpec{
			Image:    "nginx:alpine",
			Security: types_SecurityProfile{AllowRoot: true},
		})
		if cfg.User != "" {
			t.Errorf("AllowRoot=true should leave User empty for image default; got %q", cfg.User)
		}
	})

	t.Run("explicit AllowRoot+root user honored", func(t *testing.T) {
		cfg, _ := applySecurityProfile(runtime.ContainerSpec{
			Image:    "nginx:alpine",
			Security: types_SecurityProfile{User: "root", AllowRoot: true},
		})
		if cfg.User != "root" {
			t.Errorf("got %q, want 'root'", cfg.User)
		}
	})
}

// type_SecurityProfile is an alias to keep test bodies tidy.
type types_SecurityProfile = security.SecurityProfile

func containsString(xs []string, s string) bool {
	return slices.Contains(xs, s)
}
