package ingress

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// startTCPEchoBackend brings up an in-process TCP echo server on a
// random port and returns its listen port. Closes when ctx cancels.
func startTCPEchoBackend(t *testing.T, ctx context.Context) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func TestL4TCPForwarderEchoLoopback(t *testing.T) {
	ctx := t.Context()

	backendPort := startTCPEchoBackend(t, ctx)

	pools := newPoolRegistry()
	svc := ServiceID{Project: "default", Service: "echo"}
	pools.Upsert(svc, []Backend{
		{ContainerID: "b1", IPAddress: "127.0.0.1", Port: backendPort, Healthy: true},
	})

	// Pick a free port for the forwarder.
	tmp, _ := net.Listen("tcp", "127.0.0.1:0")
	fwdPort := tmp.Addr().(*net.TCPAddr).Port
	tmp.Close()

	logger := slog.New(slog.DiscardHandler)
	fwd := newL4Forwarder("127.0.0.1:"+itoa(fwdPort), "tcp", svc, "", pools, logger)

	done := make(chan error, 1)
	go func() { done <- fwd.Start(ctx) }()
	defer fwd.Stop()

	// Give the listener a moment.
	time.Sleep(50 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+itoa(fwdPort), 2*time.Second)
	if err != nil {
		t.Fatalf("dial forwarder: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello-l4-echo")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(payload) {
		t.Errorf("echo mismatch: got %q, want %q", buf, payload)
	}
}

func TestL4TCPForwarderNoHealthyBackend(t *testing.T) {
	ctx := t.Context()

	pools := newPoolRegistry()
	svc := ServiceID{Project: "default", Service: "echo"}
	pools.Upsert(svc, []Backend{
		{ContainerID: "b1", IPAddress: "127.0.0.1", Port: 1, Healthy: false}, // unhealthy
	})

	tmp, _ := net.Listen("tcp", "127.0.0.1:0")
	fwdPort := tmp.Addr().(*net.TCPAddr).Port
	tmp.Close()

	logger := slog.New(slog.DiscardHandler)
	fwd := newL4Forwarder("127.0.0.1:"+itoa(fwdPort), "tcp", svc, "", pools, logger)
	go fwd.Start(ctx)
	defer fwd.Stop()
	time.Sleep(50 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+itoa(fwdPort), 2*time.Second)
	if err != nil {
		t.Fatalf("dial forwarder: %v", err)
	}
	defer conn.Close()
	// Connection should close immediately (no backend → handleTCPConn returns).
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err == nil && n > 0 {
		t.Errorf("expected closed/EOF, got %d bytes: %q", n, buf[:n])
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	sign := ""
	if i < 0 {
		sign = "-"
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return sign + string(buf[pos:])
}
