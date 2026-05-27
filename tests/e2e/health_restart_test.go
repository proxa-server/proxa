//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_002_2_FailedProbeRestartCycle verifies the probe-driven
// restart machinery: deploy a service with an HTTP probe, freeze the
// workload inside the container so probes time out, and assert the
// reconciler replaces the container within interval × retries + grace.
func TestSC_002_2_FailedProbeRestartCycle(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-002.2")
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

	tomlPath := filepath.Join(dir, "restart.toml")
	tomlContent := `
name     = "restartsvc"
image    = "traefik/whoami:latest"
replicas = 1

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
		_ = exec.Command("docker", "rm", "-f", "proxa-default-restartsvc-0").Run()
	})

	harness.WaitForCount(t, "restartsvc", 1, 20*time.Second)

	// Grab the initial container ID — we'll watch for it to change.
	initialID := containerID(t, "proxa-default-restartsvc-0")
	if initialID == "" {
		t.Fatal("could not resolve initial container ID")
	}

	// Freeze the workload so probes time out. SIGSTOP on PID 1 leaves
	// the container in state=running (so it stays in the actual slot)
	// but the HTTP probe will fail — exercising the FR-006 path.
	if out, err := exec.Command("docker", "exec", "proxa-default-restartsvc-0", "kill", "-STOP", "1").CombinedOutput(); err != nil {
		t.Fatalf("docker exec kill -STOP: %v\n%s", err, out)
	}

	// interval=2s × retries=2 = 4s to mark unhealthy, then one tick (5s)
	// for the reconciler to act. Give it 20s with a generous margin.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		nowID := containerID(t, "proxa-default-restartsvc-0")
		if nowID != "" && nowID != initialID {
			return // SC-002-2 satisfied: replica was rotated
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("container %s never replaced within 20s after SIGSTOP", initialID)
}

func containerID(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-a",
		"--filter", "name="+name,
		"--format", "{{.ID}}").Output()
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(bytes.TrimSpace(out)))
	if i := strings.IndexByte(id, '\n'); i > 0 {
		id = id[:i]
	}
	return id
}
