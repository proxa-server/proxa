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

// TestSC_002_4_PartialDegradeAndRecovery covers a 3-replica service
// where one replica's probe fails: status transitions to "degraded"
// (SC-002-4 + SC-003), then back to "healthy" once the reconciler
// rotates the bad replica.
func TestSC_002_4_PartialDegradeAndRecovery(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-002.4")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	harness.SkipIfHTTPProbeUnreachable(t)

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "partial.toml")
	tomlContent := `
name     = "partial"
image    = "traefik/whoami:latest"
replicas = 3

[[expose]]
container = 80
host      = 0
protocol  = "http"

[health]
path     = "/health"
port     = 80
interval = "2s"
timeout  = "1s"
retries  = 2
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		for i := 0; i < 3; i++ {
			_ = exec.Command("docker", "rm", "-f", containerName("partial", i)).Run()
		}
	})

	harness.WaitForCount(t, "partial", 3, 30*time.Second)
	if !harness.WaitForServiceStatus(t, dir, "partial", "healthy", 30*time.Second) {
		out, _ := harness.RunProxa(t, dir, "ps", "-j")
		t.Fatalf("partial never reached healthy initially; last ps:\n%s", out)
	}

	// Freeze replica 1 only — service should degrade but not fail.
	if out, err := exec.Command("docker", "exec", "proxa-default-partial-1", "kill", "-STOP", "1").CombinedOutput(); err != nil {
		t.Fatalf("docker exec kill -STOP: %v\n%s", err, out)
	}

	if !harness.WaitForServiceStatus(t, dir, "partial", "degraded", 20*time.Second) {
		out, _ := harness.RunProxa(t, dir, "ps", "-j")
		t.Fatalf("partial never reached degraded after freezing replica 1; last ps:\n%s", out)
	}

	// Reconciler should rotate the bad replica and recover.
	if !harness.WaitForServiceStatus(t, dir, "partial", "healthy", 45*time.Second) {
		out, _ := harness.RunProxa(t, dir, "ps", "-j")
		t.Fatalf("partial never recovered to healthy after rotation; last ps:\n%s", out)
	}
}

func containerName(service string, replica int) string {
	switch replica {
	case 0:
		return "proxa-default-" + service + "-0"
	case 1:
		return "proxa-default-" + service + "-1"
	case 2:
		return "proxa-default-" + service + "-2"
	}
	return "proxa-default-" + service + "-?"
}
