//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_004_LogsSinceFilter covers US4 / SC-004-adjacent: --since DUR
// returns only the lines produced within the window. We deploy nginx
// (logs each request), trigger one request, sleep 5s, trigger another,
// then `proxa logs <svc> --since 3s` should return only the second.
func TestSC_004_LogsSinceFilter(t *testing.T) {
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

	hostPort, _ := harness.PickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "logsince.toml")
	tomlContent := fmt.Sprintf(`
name     = "logsince"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 1

[security]
user = "101:101"

[[expose]]
container = 8080
host      = %d
protocol  = "http"
`, hostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-logsince-0").Run()
	})
	harness.WaitForCount(t, "logsince", 1, 30*time.Second)

	// First request — tagged so we can grep for it.
	if err := exec.Command("curl", "-sf", "-A", "first-request",
		fmt.Sprintf("http://127.0.0.1:%d/", hostPort)).Run(); err != nil {
		t.Fatalf("curl 1: %v", err)
	}

	// Wait so the next request lands outside a 3s --since window.
	time.Sleep(5 * time.Second)

	// Second request.
	if err := exec.Command("curl", "-sf", "-A", "second-request",
		fmt.Sprintf("http://127.0.0.1:%d/", hostPort)).Run(); err != nil {
		t.Fatalf("curl 2: %v", err)
	}
	time.Sleep(1 * time.Second) // let nginx flush

	out, err := harness.RunProxa(t, dir, "logs", "logsince", "--since", "3s")
	if err != nil {
		t.Fatalf("logs --since 3s: %v\n%s", err, out)
	}
	hasFirst := strings.Contains(out, "first-request")
	hasSecond := strings.Contains(out, "second-request")
	if hasFirst {
		t.Errorf("--since 3s should EXCLUDE the older first-request line; output:\n%s", out)
	}
	if !hasSecond {
		t.Errorf("--since 3s should INCLUDE the recent second-request line; output:\n%s", out)
	}
}
