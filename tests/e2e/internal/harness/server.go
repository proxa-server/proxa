package harness

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// StartServer spawns `proxa server` in the background and returns a
// cancel function the test should defer. Wraps a 500ms settle so
// callers can immediately dial the socket without race.
func StartServer(t *testing.T, dir string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, ProxaBinary(t), "server")
	cmd.Env = append(os.Environ(), ProxaEnv(t, dir)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start server: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	return func() {
		cancel()
		_ = cmd.Wait()
	}
}

// ProxaEnv builds the env slice (PROXA_DATA_DIR + PROXA_LISTEN) for a
// proxa subprocess. Uses a short Unix socket path under /tmp because
// macOS caps sun_path at 104 bytes — t.TempDir()'s /var/folders/...
// path easily blows past that.
func ProxaEnv(t *testing.T, dir string) []string {
	t.Helper()
	socket := SocketPath(t, dir)
	return []string{
		"PROXA_DATA_DIR=" + dir,
		"PROXA_LISTEN=unix://" + socket,
	}
}
