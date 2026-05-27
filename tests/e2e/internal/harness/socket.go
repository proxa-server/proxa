package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// SocketPath derives a short, deterministic Unix-socket path from a
// data dir. Same dir → same socket within a test run. On macOS we
// force /tmp because $TMPDIR exceeds sun_path's 104-byte cap.
func SocketPath(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.Sum256([]byte(dir))
	id := hex.EncodeToString(h[:6])
	tmp := os.TempDir()
	if runtime.GOOS == "darwin" {
		tmp = "/tmp"
	}
	return filepath.Join(tmp, "proxa-e2e-"+id+".sock")
}

// GetViaSocket runs an HTTP GET via the Unix socket and returns the body.
// Fatals on non-200.
func GetViaSocket(t *testing.T, dir, token, path string) string {
	t.Helper()
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", SocketPath(t, dir))
			},
		},
	}
	req, _ := http.NewRequest(http.MethodGet, "http://x"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// PostViaSocketStatus runs an HTTP POST via the Unix socket and
// returns the response status code. Does NOT fatal on non-2xx — the
// caller asserts the expected status. Body is discarded; callers that
// need a body should use the lower-level pattern from GetViaSocket.
func PostViaSocketStatus(t *testing.T, dir, token, path string) int {
	t.Helper()
	return doViaSocketStatus(t, dir, token, http.MethodPost, path)
}

// DeleteViaSocketStatus runs an HTTP DELETE via the Unix socket and
// returns the response status code. Does NOT fatal on non-2xx.
func DeleteViaSocketStatus(t *testing.T, dir, token, path string) int {
	t.Helper()
	return doViaSocketStatus(t, dir, token, http.MethodDelete, path)
}

func doViaSocketStatus(t *testing.T, dir, token, method, path string) int {
	t.Helper()
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", SocketPath(t, dir))
			},
		},
	}
	req, _ := http.NewRequest(method, "http://x"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// WriteFile is a tiny os.WriteFile wrapper that makes tests less
// verbose (writes with mode 0o600).
func WriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// SSERequest opens a long-lived streaming request via the Unix socket
// and returns the live response (caller closes resp.Body). The Accept
// header is set to text/event-stream so the server takes the SSE branch.
func SSERequest(t *testing.T, dir, token, path string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{
		// NO timeout — streaming.
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", SocketPath(t, dir))
			},
		},
	}
	req, _ := http.NewRequest(http.MethodGet, "http://x"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE GET %s: %v", path, err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("SSE GET %s: status %d", path, resp.StatusCode)
	}
	return resp, ""
}
