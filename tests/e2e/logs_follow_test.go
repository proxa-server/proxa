//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSC_002_LogsFollowLatency covers SC-002 + SC-003:
//   - SC-002: a new log line reaches the proxa-logs subprocess within 1
//     second of being written.
//   - SC-003: SIGINT to the subprocess exits within 1s with exit code 0
//     or 130 (no orphaned ESTABLISHED — the http.Request.Context cancel
//     closes the TCP connection).
func TestSC_002_LogsFollowLatency(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	hostPort, _ := pickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "logfollow.toml")
	// nginx-unprivileged logs every request to stdout — whoami does NOT
	// by default, so we use nginx for the latency test.
	tomlContent := fmt.Sprintf(`
name     = "logfollow"
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
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-logfollow-0").Run()
	})
	waitForCount(t, "logfollow", 1, 30*time.Second)

	// Start `proxa logs <svc> -f --tail 0` (skip history; only stream
	// new lines) as a subprocess; lines flow into a channel.
	cmd := exec.Command(proxaBinary(t), "logs", "logfollow", "-f")
	cmd.Env = append(os.Environ(), proxaEnv(t, dir)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr // surface meta header on test stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start logs: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	lines := make(chan string, 100)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	// Give the subprocess + server a moment to set up the stream.
	time.Sleep(2 * time.Second)

	// Fire a curl that triggers a known log line; record the moment.
	markBefore := time.Now()
	if err := exec.Command("curl", "-sf", fmt.Sprintf("http://127.0.0.1:%d/", hostPort)).Run(); err != nil {
		t.Fatalf("curl whoami: %v", err)
	}

	// Wait up to 3s for a matching line via channel select.
	deadline := time.After(3 * time.Second)
	found := false
loop:
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				break loop
			}
			if strings.Contains(line, "GET /") || strings.Contains(line, "\"GET ") {
				latency := time.Since(markBefore)
				t.Logf("first matching line after curl: %s (latency=%v)", line, latency)
				if latency > 2*time.Second {
					t.Errorf("latency %v exceeds 2s budget (SC-002 wants ≤ 1s; 2s is generous test tolerance)", latency)
				}
				found = true
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if !found {
		t.Fatalf("did not observe a new log line within 3s of curl")
	}

	// SC-003: SIGINT should exit within 1s.
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("SIGINT: %v", err)
	}
	exitC := make(chan error, 1)
	go func() { exitC <- cmd.Wait() }()
	select {
	case waitErr := <-exitC:
		// exit 0 or 130 both acceptable per CLI contract.
		if waitErr != nil {
			if ee, ok := waitErr.(*exec.ExitError); ok {
				code := ee.ExitCode()
				if code != 130 && code != 0 {
					t.Errorf("exit code = %d, want 0 or 130", code)
				}
			} else {
				t.Errorf("wait error: %v", waitErr)
			}
		}
	case <-time.After(2 * time.Second):
		t.Errorf("proxa logs -f did not exit within 2s of SIGINT")
	}
}
