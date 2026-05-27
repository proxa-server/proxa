//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_008_DashboardRendersServices verifies the slim dashboard
// renders the services table when accessed over the Unix socket
// (which bypasses Bearer-token auth in v0.0).
func TestSC_008_DashboardRendersServices(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-008")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "dash.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"dashtest\"\nimage = \"traefik/whoami:latest\"\nreplicas = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	harness.WaitForCount(t, "dashtest", 2, 15*time.Second)

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f",
			"proxa-default-dashtest-0", "proxa-default-dashtest-1").Run()
	})

	body := getViaUnixSocket(t, harness.SocketPath(t, dir), "/ui/")
	for _, want := range []string{"<title>Proxa", "dashtest", "traefik/whoami:latest"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard HTML missing %q\n--- snippet ---\n%s", want, harness.Snippet(body, 400))
		}
	}

	// Fragment endpoint that HTMX polls.
	frag := getViaUnixSocket(t, harness.SocketPath(t, dir), "/ui/services")
	if !strings.Contains(frag, "dashtest") {
		t.Errorf("services fragment missing dashtest")
	}
}

// getViaUnixSocket dials the Unix socket and performs an HTTP GET.
func getViaUnixSocket(t *testing.T, sock, path string) string {
	t.Helper()
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get("http://x" + path)
	if err != nil {
		t.Fatalf("GET %s via unix socket: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	buf := make([]byte, 64*1024)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}
