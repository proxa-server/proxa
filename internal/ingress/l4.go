package ingress

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

// l4Forwarder owns one TCP-or-UDP listener and forwards connections /
// packets to a backend chosen from the BackendPool at accept time.
//
// TCP: per-connection backend pin (R-004). Once a backend is chosen at
// Accept, the goroutine that pumps bytes uses that backend until either
// side closes — no mid-connection rebalancing.
//
// UDP: per-source-address sticky for 30s (R-004). Stateless protocols
// (DNS, QUIC handshake) get backend continuity without server-side state.
type l4Forwarder struct {
	listenAddr string // ":18379"
	proto      string // "tcp" or "udp"
	svc        ServiceID
	lb         string
	pools      *poolRegistry
	logger     *slog.Logger

	mu     sync.Mutex
	tcpLis net.Listener
	udpCon net.PacketConn

	udpMu    sync.Mutex
	udpRoute map[string]udpStickyEntry // src "ip:port" → backend choice
}

type udpStickyEntry struct {
	backend Backend
	expires time.Time
}

func newL4Forwarder(addr, proto string, svc ServiceID, lb string, pools *poolRegistry, logger *slog.Logger) *l4Forwarder {
	return &l4Forwarder{
		listenAddr: addr,
		proto:      proto,
		svc:        svc,
		lb:         lb,
		pools:      pools,
		logger:     logger,
		udpRoute:   make(map[string]udpStickyEntry),
	}
}

// Start binds the listener and serves until ctx cancels.
func (f *l4Forwarder) Start(ctx context.Context) error {
	switch f.proto {
	case "tcp":
		return f.startTCP(ctx)
	case "udp":
		return f.startUDP(ctx)
	default:
		return fmt.Errorf("ingress/l4: unknown proto %q", f.proto)
	}
}

// Stop closes the listener; the accept/serve loop in Start returns soon after.
func (f *l4Forwarder) Stop() {
	f.mu.Lock()
	tcpLis, udpCon := f.tcpLis, f.udpCon
	f.mu.Unlock()
	if tcpLis != nil {
		_ = tcpLis.Close()
	}
	if udpCon != nil {
		_ = udpCon.Close()
	}
}

func (f *l4Forwarder) startTCP(ctx context.Context) error {
	l, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		return fmt.Errorf("ingress/l4: tcp listen %s: %w", f.listenAddr, err)
	}
	f.mu.Lock()
	f.tcpLis = l
	f.mu.Unlock()
	f.logger.Info("ingress: L4 TCP listener", "addr", f.listenAddr, "service", f.svc.Project+"/"+f.svc.Service)

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			f.logger.Warn("ingress/l4: accept failed", "addr", f.listenAddr, "err", err)
			continue
		}
		go f.handleTCPConn(ctx, conn)
	}
}

func (f *l4Forwarder) handleTCPConn(ctx context.Context, client net.Conn) {
	defer client.Close()
	pool := f.pools.Get(f.svc)
	if pool == nil {
		return // no pool yet; drop the connection
	}
	backend := pool.Pick(f.lb)
	if backend == nil {
		return // no healthy backend; drop
	}
	backendAddr := fmt.Sprintf("%s:%d", backend.IPAddress, backend.Port)
	upstream, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		f.logger.Warn("ingress/l4: backend dial failed", "addr", backendAddr, "err", err)
		return
	}
	defer upstream.Close()

	// Bidirectional copy.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (f *l4Forwarder) startUDP(ctx context.Context) error {
	pc, err := net.ListenPacket("udp", f.listenAddr)
	if err != nil {
		return fmt.Errorf("ingress/l4: udp listen %s: %w", f.listenAddr, err)
	}
	f.mu.Lock()
	f.udpCon = pc
	f.mu.Unlock()
	f.logger.Info("ingress: L4 UDP listener", "addr", f.listenAddr, "service", f.svc.Project+"/"+f.svc.Service)

	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()

	buf := make([]byte, 64*1024)
	for {
		n, src, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			f.logger.Warn("ingress/l4: udp read failed", "err", err)
			continue
		}
		backend, ok := f.pickUDPBackend(src.String())
		if !ok {
			continue
		}
		// Fire-and-forget forward to backend. We don't currently route
		// the response back to src — that lands when a real UDP workload
		// asks for it; for v0.3 SC-006 (TCP) is the only L4 SC.
		go f.forwardUDP(backend, append([]byte(nil), buf[:n]...))
	}
}

func (f *l4Forwarder) pickUDPBackend(srcKey string) (Backend, bool) {
	f.udpMu.Lock()
	defer f.udpMu.Unlock()
	if ent, ok := f.udpRoute[srcKey]; ok && time.Now().Before(ent.expires) {
		return ent.backend, true
	}
	pool := f.pools.Get(f.svc)
	if pool == nil {
		return Backend{}, false
	}
	b := pool.Pick(f.lb)
	if b == nil {
		return Backend{}, false
	}
	f.udpRoute[srcKey] = udpStickyEntry{backend: *b, expires: time.Now().Add(30 * time.Second)}
	return *b, true
}

func (f *l4Forwarder) forwardUDP(backend Backend, payload []byte) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", backend.IPAddress, backend.Port))
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write(payload)
}
