//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_004_ProxaDownRemovesContainers verifies that proxa down
// scales the service to zero and the reconciler removes its
// containers within one tick interval.
func TestSC_004_ProxaDownRemovesContainers(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-004")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "downtest.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"downtest\"\nimage = \"traefik/whoami:latest\"\nreplicas = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	harness.WaitForCount(t, "downtest", 2, 15*time.Second)

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f",
			"proxa-default-downtest-0", "proxa-default-downtest-1").Run()
	})

	if out, err := harness.RunProxa(t, dir, "down", "downtest"); err != nil {
		t.Fatalf("down: %v\n%s", err, out)
	}
	harness.WaitForCount(t, "downtest", 0, 15*time.Second)
}

// TestProxaDownIsIdempotent verifies that down-on-missing-service
// exits 0 (per the Client.Scale 404-tolerant contract).
func TestProxaDownIsIdempotent(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	if out, err := harness.RunProxa(t, dir, "down", "does-not-exist"); err != nil {
		t.Errorf("down on missing service should succeed; got: %v\n%s", err, out)
	}
}
