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

// TestSC_002_3_ExecProbeHealthyAndFailing covers exec probes end-to-end:
//
//   - Deploy a service with an exec probe that succeeds (redis-cli ping).
//     Status reaches "healthy" (SC-002-3).
//   - Flip the TOML to a probe that always fails ("false"). The
//     reconciler rotates the container — its ID changes (SC-004).
//
// We use redis:7-alpine because it runs a long-lived server AND ships
// the busybox userspace (true/false/redis-cli all reachable via docker
// exec). traefik/whoami is from scratch, so docker exec into it has
// nothing to run besides /whoami itself.
func TestSC_002_3_ExecProbeHealthyAndFailing(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "exec.toml")
	goodTOML := `
name     = "execsvc"
image    = "redis:7-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[health]
command  = ["redis-cli", "ping"]
interval = "2s"
timeout  = "1s"
retries  = 3
`
	if err := os.WriteFile(tomlPath, []byte(goodTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-execsvc-0").Run()
	})

	waitForCount(t, "execsvc", 1, 30*time.Second)

	// SC-002-3: status reaches "healthy".
	if !waitForServiceStatus(t, dir, "execsvc", "healthy", 30*time.Second) {
		out, _ := runProxa(t, dir, "ps", "-j")
		t.Fatalf("execsvc never reached healthy; last ps:\n%s", out)
	}

	// SC-004: flip to a failing probe. The new spec changes spec_hash so
	// the reconciler triggers a Replace via stop-first — container ID
	// changes regardless of whether the new probe ever passes.
	initialID := containerID(t, "proxa-default-execsvc-0")
	badTOML := `
name     = "execsvc"
image    = "redis:7-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[health]
command  = ["false"]
interval = "2s"
timeout  = "1s"
retries  = 2
`
	if err := os.WriteFile(tomlPath, []byte(badTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up (flip): %v\n%s", err, out)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		nowID := containerID(t, "proxa-default-execsvc-0")
		if nowID != "" && nowID != initialID {
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("execsvc never rotated after flipping to a failing exec probe")
}
