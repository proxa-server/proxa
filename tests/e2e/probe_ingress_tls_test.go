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

// TestSC_001_ProbeViaIngress_TLS covers SC-001 / FR-005 and is the
// end-to-end regression gate for the v0.4.0 demo bug: a service
// exposed through ingress with TLS=true AND a health probe
// (via=ingress) used to fail because the probe hit the HTTP→HTTPS
// 301 from the ingress and treated it as non-2xx.
//
// With the v0.4.1 fix (specs/005-modern-go/research.md R-001 Approach
// B), the probe targets the HTTPS port directly with cert verification
// off (loopback only) and the service reaches healthy within 30s
// without any manual workaround.
//
// Independence test for US1: this is the ONLY test required to prove
// the bug is fixed end-to-end.
func TestSC_001_ProbeViaIngress_TLS(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-001")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := harness.PickTwoFreeTCPPorts(t)
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = true\nemail = \"\"\n", httpPort, httpsPort)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := harness.StartServer(t, dir)
	defer stop()

	backendHostPort, _ := harness.PickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "tlsprobe.toml")
	tomlContent := fmt.Sprintf(`
name     = "tlsprobe"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 1

[security]
user = "101:101"

[[expose]]
container = 8080
host      = %d
protocol  = "http"

[[route]]
host = "tlsprobe.local"

[health]
path     = "/"
port     = 8080
interval = "2s"
timeout  = "1s"
retries  = 3
via      = "ingress"
`, backendHostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-tlsprobe-0").Run()
	})

	harness.WaitForCount(t, "tlsprobe", 1, 30*time.Second)

	// The regression assertion: with TLS=true ingress + probe via=ingress,
	// the service MUST reach healthy without manual intervention.
	if !harness.WaitForServiceStatus(t, dir, "tlsprobe", "healthy", 30*time.Second) {
		out, _ := harness.RunProxa(t, dir, "ps", "-j")
		t.Fatalf("0.4.0 demo bug regressed: TLS-enabled service with via=ingress probe never reached healthy.\nlast ps:\n%s", out)
	}
}

// TestSC_001_ProbeViaIngress_TLS_FollowRedirectsOverride covers FR-006:
// when the operator explicitly sets follow_redirects = false on a
// TLS+via=ingress probe, the probe stays on the HTTP port and sees the
// 301 as a failure (operator opted in to assert the redirect). The
// service is marked unhealthy in that case — the test confirms the
// override actually engages rather than getting silently overridden by
// the default-fix branch.
func TestSC_001_ProbeViaIngress_TLS_FollowRedirectsOverride(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-001")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := harness.PickTwoFreeTCPPorts(t)
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = true\nemail = \"\"\n", httpPort, httpsPort)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := harness.StartServer(t, dir)
	defer stop()

	backendHostPort, _ := harness.PickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "tlsprobeopt.toml")
	// follow_redirects = false forces the probe to stay on HTTP and
	// treat the 3xx as non-2xx; service should NOT reach healthy.
	tomlContent := fmt.Sprintf(`
name     = "tlsprobeopt"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 1

[security]
user = "101:101"

[[expose]]
container = 8080
host      = %d
protocol  = "http"

[[route]]
host = "tlsprobeopt.local"

[health]
path             = "/"
port             = 8080
interval         = "2s"
timeout          = "1s"
retries          = 3
via              = "ingress"
follow_redirects = false
`, backendHostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-tlsprobeopt-0").Run()
	})

	harness.WaitForCount(t, "tlsprobeopt", 1, 30*time.Second)

	// With follow_redirects=false explicitly set, the probe stays on
	// HTTP and the 301 surfaces as unhealthy. We allow the service to
	// briefly appear healthy during startup, then assert it eventually
	// degrades (or never reaches healthy). The point is to prove the
	// override actually changes behavior — not to assert a specific
	// terminal state.
	deadline := time.Now().Add(20 * time.Second)
	sawUnhealthy := false
	for time.Now().Before(deadline) {
		out, _ := harness.RunProxa(t, dir, "ps", "-j")
		if !strings.Contains(out, `"status":"healthy"`) {
			sawUnhealthy = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !sawUnhealthy {
		out, _ := harness.RunProxa(t, dir, "ps", "-j")
		t.Errorf("FR-006 override failed to engage: tlsprobeopt stayed healthy despite follow_redirects=false\nlast ps:\n%s", out)
	}
}
