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

// TestFR_007_LogsIconInServicesAndRoutesTables covers FR-007 dashboard
// parity: every Services row AND every Routes row exposes a "📜 logs"
// link pointing at /ui/logs/{project}/{service}.
//
// Without this regression gate, a future change to the table templates
// could silently strip the icon and the operator's dashboard debug loop
// breaks without a failing test.
func TestFR_007_LogsIconInServicesAndRoutesTables(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := harness.PickTwoFreeTCPPorts(t)
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = false\n", httpPort, httpsPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := harness.StartServer(t, dir)
	defer stop()

	// Deploy a service WITH a route so both tables have a row to verify.
	hostPort, _ := harness.PickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "parity.toml")
	tomlContent := fmt.Sprintf(`
name     = "parity"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 1

[security]
user = "101:101"

[[expose]]
container = 8080
host      = %d
protocol  = "http"

[[route]]
host = "parity.local"
`, hostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-parity-0").Run()
	})
	harness.WaitForCount(t, "parity", 1, 30*time.Second)
	// Wait a tick more so the reconciler pushes routes/backends to ingress
	// and the route table builder sees the new row.
	time.Sleep(6 * time.Second)

	token := harness.ReadToken(t, dir)
	want := `href="/ui/logs/default/parity"`

	// Services card: every service row must include the logs link.
	svcBody := harness.GetViaSocket(t, dir, token, "/ui/services")
	if !strings.Contains(svcBody, want) {
		t.Errorf("/ui/services missing logs link %q\n--- snippet ---\n%s", want, harness.Snippet(svcBody, 1200))
	}
	if !strings.Contains(svcBody, "📜 logs") {
		t.Errorf("/ui/services missing 📜 logs icon text")
	}

	// Routes card: every route row must include the logs link.
	routesBody := harness.GetViaSocket(t, dir, token, "/ui/routes")
	if !strings.Contains(routesBody, want) {
		t.Errorf("/ui/routes missing logs link %q\n--- snippet ---\n%s", want, harness.Snippet(routesBody, 1200))
	}
	if !strings.Contains(routesBody, "📜 logs") {
		t.Errorf("/ui/routes missing 📜 logs icon text")
	}

	// And the target page must actually exist (no broken-link surprise).
	logsBody := harness.GetViaSocket(t, dir, token, "/ui/logs/default/parity")
	if !strings.Contains(logsBody, "logsController") {
		t.Errorf("/ui/logs/default/parity didn't render the viewer (logsController not found)\n%s", harness.Snippet(logsBody, 400))
	}
}
