//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
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

// TestSC_002_IngressLBAcrossReplicas covers SC-002: a service with
// replicas=3 distributes inbound requests across all 3 backends.
//
// Skipped on Docker Desktop hosts: multi-replica services with one
// shared host port can only land on bridge IPs (host port = one slot,
// one bind), and the bridge subnet is unreachable from the host on
// Docker Desktop. On a Linux server this test runs normally.
func TestSC_002_IngressLBAcrossReplicas(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	skipIfHTTPProbeUnreachable(t)

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := pickTwoFreeTCPPorts(t)
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = true\nemail = \"\"\n", httpPort, httpsPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "lb.toml")
	tomlContent := `
name     = "lbsvc"
image    = "traefik/whoami:latest"
replicas = 3

[[expose]]
container = 80
host      = 0
protocol  = "http"

[[route]]
host = "lb.local"
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		for i := 0; i < 3; i++ {
			_ = exec.Command("docker", "rm", "-f", fmt.Sprintf("proxa-default-lbsvc-%d", i)).Run()
		}
	})

	waitForCount(t, "lbsvc", 3, 45*time.Second)
	if !waitForServiceStatus(t, dir, "lbsvc", "healthy", 30*time.Second) {
		t.Fatalf("lbsvc never reached healthy")
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "lb.local"},
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", httpsPort))
			},
		},
	}

	hostnames := map[string]int{}
	for i := 0; i < 30; i++ {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://lb.local:%d/", httpsPort), nil)
		req.Host = "lb.local"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		resp.Body.Close()
		for _, line := range strings.Split(string(buf[:n]), "\n") {
			if strings.HasPrefix(line, "Hostname:") {
				hostnames[strings.TrimSpace(strings.TrimPrefix(line, "Hostname:"))]++
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(hostnames) < 2 {
		t.Errorf("expected ≥ 2 distinct backends across 30 requests, got %d: %v", len(hostnames), hostnames)
	}
}
