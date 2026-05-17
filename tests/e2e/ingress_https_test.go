//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSC_001_IngressHTTPSAndDashboard covers SC-001 in self-signed
// mode (TLS=true, Email=""): deploy a whoami service with a [[route]]
// block, verify https://<host>:<port>/ returns 200 over TLS, AND
// verify the dashboard's /api/v1/routes endpoint + /ui/routes HTML
// fragment surface the new route. Self-signed mode bypasses Let's
// Encrypt rate limits.
//
// This is also the gate that proves the dashboard work landed
// alongside the feature (per user instruction: "we'd be destroying
// with one hand what we build with the other" if dashboard lagged).
func TestSC_001_IngressHTTPSAndDashboard(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	// Note: NOT using skipIfHTTPProbeUnreachable here — the test deploys
	// the workload with [[expose]] host > 0 so the ingress dials
	// 127.0.0.1:<hostport> instead of the bridge IP. Works on macOS
	// Docker Desktop without VM-side routability.

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	// Pick unused TCP ports for the ingress + backend so the test is hermetic.
	httpPort, httpsPort := pickTwoFreeTCPPorts(t)
	backendHostPort, _ := pickTwoFreeTCPPorts(t) // need only one
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := fmt.Sprintf(`
[ingress]
http_port  = %d
https_port = %d
tls        = true
email      = ""
`, httpPort, httpsPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "whoami.toml")
	tomlContent := fmt.Sprintf(`
name     = "whoami"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = %d
protocol  = "http"

[[route]]
host = "whoami.local"
`, backendHostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-whoami-0").Run()
	})

	waitForCount(t, "whoami", 1, 30*time.Second)
	if !waitForServiceStatus(t, dir, "whoami", "healthy", 30*time.Second) {
		t.Fatalf("whoami service never reached healthy")
	}

	// --- Part 1: HTTPS reachability via the ingress with self-signed cert ---

	// Wait up to 15s for ingress to bind + reconciler to publish the route.
	httpsURL := fmt.Sprintf("https://127.0.0.1:%d/", httpsPort)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "whoami.local"},
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", httpsPort))
			},
		},
	}
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	var lastStatus int
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, httpsURL, nil)
		req.Host = "whoami.local"
		resp, err := client.Do(req)
		if err == nil {
			lastStatus = resp.StatusCode
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				goto httpsOK
			}
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("HTTPS request to %s never succeeded; lastStatus=%d lastErr=%v", httpsURL, lastStatus, lastErr)

httpsOK:

	// --- Part 2: dashboard JSON shows the new route ---

	out, err := runProxa(t, dir, "ps", "-j")
	if err != nil {
		t.Fatalf("ps -j: %v\n%s", err, out)
	}
	// /api/v1/routes via curl since proxa ps doesn't include routes (yet).
	apiOut, err := exec.Command("curl", "-sS", "--unix-socket", socketPath(t, dir),
		"-H", "Authorization: Bearer "+readToken(t, dir),
		"http://x/api/v1/routes").Output()
	if err != nil {
		t.Fatalf("GET /api/v1/routes: %v", err)
	}
	var routesResp struct {
		Total  int `json:"total"`
		Routes []struct {
			Host         string `json:"Host"`
			Service      string `json:"Service"`
			TLSStatus    string `json:"TLSStatus"`
			BackendCount int    `json:"BackendCount"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(apiOut, &routesResp); err != nil {
		t.Fatalf("parse /api/v1/routes JSON: %v\nbody=%s", err, apiOut)
	}
	if routesResp.Total != 1 {
		t.Errorf("/api/v1/routes total = %d, want 1\nbody=%s", routesResp.Total, apiOut)
	}
	found := false
	for _, r := range routesResp.Routes {
		if r.Host == "whoami.local" && r.Service == "whoami" {
			found = true
			if r.BackendCount != 1 {
				t.Errorf("route backend count = %d, want 1", r.BackendCount)
			}
			if r.TLSStatus != "valid" {
				t.Errorf("route TLSStatus = %q, want valid (self-signed mode)", r.TLSStatus)
			}
		}
	}
	if !found {
		t.Errorf("/api/v1/routes missing whoami.local → whoami\nbody=%s", apiOut)
	}

	// --- Part 3: /ui/routes HTML fragment contains the new route ---

	uiOut, err := exec.Command("curl", "-sS", "--unix-socket", socketPath(t, dir),
		"-H", "Authorization: Bearer "+readToken(t, dir),
		"http://x/ui/routes").Output()
	if err != nil {
		t.Fatalf("GET /ui/routes: %v", err)
	}
	body := string(uiOut)
	for _, want := range []string{"whoami.local", "whoami", "chip-blue", "valid"} {
		if !strings.Contains(body, want) {
			t.Errorf("/ui/routes HTML missing %q\n--- snippet ---\n%s", want, snippet(body, 600))
		}
	}
}

func readToken(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// pickTwoFreeTCPPorts returns two distinct ports that are free at the
// moment of the call. Race-y in principle (some other process can grab
// them between the test's port discovery and the server's bind) but
// fine for local CI.
func pickTwoFreeTCPPorts(t *testing.T) (int, int) {
	t.Helper()
	pick := func() int {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	return pick(), pick()
}
