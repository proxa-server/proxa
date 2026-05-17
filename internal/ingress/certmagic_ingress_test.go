package ingress

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/config"
)

// TestCertMagicIngressNonPrivilegedPorts verifies SC-008: a Run with
// HTTP port 18080 and HTTPS port 18443 (both > 1024) succeeds for a
// non-root process. We don't actually start a real workload — just
// confirm the listener accepts a TCP connection on the configured port.
func TestCertMagicIngressNonPrivilegedPorts(t *testing.T) {
	// Pick free ports above 1024 to make the test hermetic; production
	// non-root operators would set fixed numbers like 8080 / 8443.
	httpPort, httpsPort := pickFreePort(t), pickFreePort(t)

	cfg := config.IngressConfig{
		HTTPPort:  httpPort,
		HTTPSPort: httpsPort,
		TLS:       false,
	}
	ic := New(cfg, t.TempDir(), slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- ic.Run(ctx) }()

	// Wait for the listener to be ready.
	addr := net.JoinHostPort("127.0.0.1", itoa(httpPort))
	dialOK := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			dialOK = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !dialOK {
		t.Fatalf("HTTP listener on non-privileged port %d never accepted", httpPort)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned err on graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("Run did not return within 5s of ctx cancel")
	}
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
