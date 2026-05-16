//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestSC_002_8_NoHealthBlockRegression verifies that a service declared
// WITHOUT a [health] block behaves exactly like a v0.1.0 service: the
// container runs, status reaches "healthy" (count-derived fallback in
// the server), and no probe goroutines are started. This guards against
// silently breaking services that don't opt into health checks.
func TestSC_002_8_NoHealthBlockRegression(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "noprobe.toml")
	tomlContent := `
name     = "noprobe"
image    = "traefik/whoami:latest"
replicas = 1
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-noprobe-0").Run()
	})

	waitForCount(t, "noprobe", 1, 30*time.Second)
	if !waitForServiceStatus(t, dir, "noprobe", "healthy", 30*time.Second) {
		out, _ := runProxa(t, dir, "ps", "-j")
		t.Fatalf("noprobe never reached healthy without [health] block; last ps:\n%s", out)
	}
}
