package bench_test

import (
	"io"
	"net"
	"testing"
)

// BenchmarkL4_Throughput measures sustained TCP-proxy throughput. We
// don't import the internal/ingress L4 forwarder directly (it's
// package-private), but the proxy's core is just bidirectional io.Copy
// between two TCP connections — the bench reproduces that with a real
// loopback echo server. The number tracks how fast Go's io.Copy + TCP
// loopback can move bytes through the kernel, which IS the L4
// forwarder's hot path.
//
// Reports "MB/sec" sustained for a fixed payload size.
func BenchmarkL4_Throughput(b *testing.B) {
	// Echo server: accept connections, copy everything client writes
	// back to the client.
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen echo: %v", err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go io.Copy(c, c)
		}
	}()

	// Proxy server: accept client, dial echo, splice bidirectionally.
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen proxy: %v", err)
	}
	defer proxy.Close()
	go func() {
		for {
			client, err := proxy.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				upstream, err := net.Dial("tcp", echo.Addr().String())
				if err != nil {
					return
				}
				defer upstream.Close()
				done := make(chan struct{}, 2)
				go func() { io.Copy(upstream, c); done <- struct{}{} }()
				go func() { io.Copy(c, upstream); done <- struct{}{} }()
				<-done
			}(client)
		}
	}()

	const payloadSize = 64 * 1024 // 64 KiB per write — typical jumbo-ish frame
	payload := make([]byte, payloadSize)
	readBuf := make([]byte, payloadSize)

	conn, err := net.Dial("tcp", proxy.Addr().String())
	if err != nil {
		b.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	b.SetBytes(int64(payloadSize))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Write(payload); err != nil {
			b.Fatalf("write: %v", err)
		}
		if _, err := io.ReadFull(conn, readBuf); err != nil {
			b.Fatalf("read: %v", err)
		}
	}
	b.StopTimer()

	mbPerSec := (float64(b.N) * float64(payloadSize)) / b.Elapsed().Seconds() / (1024 * 1024)
	b.ReportMetric(mbPerSec, "MB/sec")
}
