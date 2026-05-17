//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSC_006_IngressTCPForward covers SC-006: deploy a redis service
// with an L4 TCP route, dial the ingress port, run a redis-cli PING,
// expect PONG.
//
// The redis backend is exposed on a host port so the ingress can dial
// 127.0.0.1:<hostPort> (works on macOS Docker Desktop without bridge
// routability). The L4 route is on a distinct ingress port.
func TestSC_006_IngressTCPForward(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := pickTwoFreeTCPPorts(t)
	ingressTCPPort, backendHostPort := pickTwoFreeTCPPorts(t)
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = false\n", httpPort, httpsPort)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "redis.toml")
	tomlContent := fmt.Sprintf(`
name     = "redisl4"
image    = "redis:7-alpine"
replicas = 1
stateful = true

[[expose]]
container = 6379
host      = %d
protocol  = "tcp"

[[route]]
host = "redis.local"
l4   = "tcp"
port = %d

[health]
command  = ["redis-cli", "ping"]
interval = "3s"
timeout  = "1s"
retries  = 3
`, backendHostPort, ingressTCPPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-redisl4-0").Run()
	})

	waitForCount(t, "redisl4", 1, 30*time.Second)
	if !waitForServiceStatus(t, dir, "redisl4", "healthy", 30*time.Second) {
		t.Fatalf("redis never reached healthy")
	}

	// Give the ingress one more reconciler tick to pick up the L4 route + backend.
	time.Sleep(8 * time.Second)

	// Speak the RESP protocol directly so the test has no external CLI
	// dependency. PING request: "*1\r\n$4\r\nPING\r\n"
	// PONG response:           "+PONG\r\n"
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ingressTCPPort), 3*time.Second)
	if err != nil {
		t.Fatalf("dial ingress L4 :%d: %v", ingressTCPPort, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		t.Fatalf("write PING: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, err := io.ReadAtLeast(conn, buf, 7) // "+PONG\r\n"
	if err != nil {
		t.Fatalf("read PONG: %v", err)
	}
	got := strings.TrimSpace(string(buf[:n]))
	if got != "+PONG" {
		t.Errorf("RESP reply via ingress = %q, want +PONG", got)
	}
}
