//go:build e2e
// +build e2e

// Run with: make test-e2e
//
// Requires Docker daemon + a built `bin/proxa` binary. These tests
// exercise the compiled binary as a subprocess against a temporary
// data dir and verify SC-001 (security defaults) end-to-end.

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

func TestSC_001_DeployWithSecurityDefaults(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-001")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	// Write a TaskDef.
	tomlPath := filepath.Join(dir, "web.toml")
	tomlContent := `
name     = "web"
image    = "nginx:alpine"
replicas = 1

[[expose]]
container = 80
host      = 0
protocol  = "http"
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}

	// Wait for reconciler to converge.
	time.Sleep(7 * time.Second)

	// Cleanup: best-effort tear down the container.
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-web-0").Run()
	})

	// SC-001: verify security defaults.
	caps := harness.DockerInspect(t, "proxa-default-web-0", "{{json .HostConfig.CapDrop}}")
	if caps == "" || caps == "null\n" {
		t.Errorf("CapDrop is empty: %q", caps)
	}
	user := harness.DockerInspect(t, "proxa-default-web-0", "{{.Config.User}}")
	if user == "" || user == "\n" || user == "root\n" || user == "0\n" {
		t.Errorf("User should be non-root, got %q", user)
	}
	secOpt := harness.DockerInspect(t, "proxa-default-web-0", "{{json .HostConfig.SecurityOpt}}")
	if !contains(secOpt, "no-new-privileges:true") {
		t.Errorf("SecurityOpt missing no-new-privileges:true, got %q", secOpt)
	}
}

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

// _ is an unused-symbol guard.
var _ = fmt.Sprintf
