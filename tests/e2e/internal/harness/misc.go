package harness

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ReadToken reads $dir/token (the bootstrap admin token written by
// `proxa init`) and returns it trimmed.
func ReadToken(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// PickTwoFreeTCPPorts returns two distinct free-at-the-moment ports.
// Race-y in principle (some other process can grab them between this
// call and the test's bind) but fine for local CI.
func PickTwoFreeTCPPorts(t *testing.T) (int, int) {
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

// Snippet returns up to n characters of s — used to make failure-output
// strings readable in test logs without dumping the whole body.
func Snippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// FindRepoRoot walks up from cwd looking for go.mod and returns the
// containing directory. Used by tests that need to copy the repo into a
// scratch dir (e.g., tool_directive_test.go).
func FindRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
