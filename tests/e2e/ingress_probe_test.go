//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestSC_007_ProbeViaIngress covers SC-007: an HTTP probe with
// Via="ingress" routes through the ingress loopback instead of the
// container's bridge IP. The test does NOT call
// skipIfHTTPProbeUnreachable — proving the macOS Docker Desktop story.
//
// On a routable Linux host this is also a smoke test that the
// loopback path works alongside the direct-dial path.
func TestSC_007_ProbeViaIngress(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := pickTwoFreeTCPPorts(t)
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = false\n", httpPort, httpsPort)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := startServer(t, dir)
	defer stop()

	backendHostPort, _ := pickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "viaingress.toml")
	tomlContent := fmt.Sprintf(`
name     = "viaingress"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = %d
protocol  = "http"

[[route]]
host = "viaingress.local"

[health]
path     = "/health"
port     = 80
interval = "2s"
timeout  = "1s"
retries  = 3
via      = "ingress"
`, backendHostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-viaingress-0").Run()
	})

	waitForCount(t, "viaingress", 1, 30*time.Second)
	if !waitForServiceStatus(t, dir, "viaingress", "healthy", 30*time.Second) {
		out, _ := runProxa(t, dir, "ps", "-j")
		t.Fatalf("viaingress service never reached healthy via ingress loopback; last ps:\n%s", out)
	}
}
