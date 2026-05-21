package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/pkg/types"
)

// Snapshot is the read-side view of a tracked container's health.
// Copy-by-value — safe to share without locking.
type Snapshot struct {
	ContainerID string
	HealthOK    bool
	LastProbeAt time.Time
	LastErr     string
	Streak      int
}

// Manager owns one goroutine per tracked container that runs HTTP/exec
// probes. The reconciler queries Snapshot() each tick to feed the
// status aggregator.
type Manager struct {
	rt  runtime.Runtime
	log *slog.Logger

	// ingressHTTPPort is the loopback port HTTPProbes target when a
	// service opts into spec.Health.Via == "ingress". Set via Options.
	// Zero value disables the ingress path; HTTPProbe falls back to the
	// direct bridge-IP dial.
	ingressHTTPPort int

	// ingressHTTPSPort is the loopback HTTPS port. When ingressTLSEnabled
	// is true and a via-ingress probe has no explicit FollowRedirects
	// override, the probe targets this port directly (instead of HTTP)
	// to avoid the 0.4.0 redirect/cert collision. Set via Options.
	ingressHTTPSPort  int
	ingressTLSEnabled bool

	mu      sync.Mutex
	entries map[string]*entry

	mgrCtx    context.Context
	mgrCancel context.CancelFunc

	wg sync.WaitGroup
}

// Options carries optional Manager configuration. Empty value yields
// v0.2 behavior (HTTPProbes always dial the bridge IP).
type Options struct {
	// IngressHTTPPort is the loopback HTTP port HTTPProbes target when
	// a service opts into spec.Health.Via == "ingress". Zero disables
	// the ingress path.
	IngressHTTPPort int

	// IngressHTTPSPort is the loopback HTTPS port. Used together with
	// IngressTLSEnabled by the v0.4.1 fix for probes against
	// TLS-enabled ingress.
	IngressHTTPSPort int

	// IngressTLSEnabled mirrors [ingress].tls. When true and a via-
	// ingress probe has no explicit follow_redirects override, the
	// probe targets IngressHTTPSPort directly to avoid the redirect
	// loop that broke probes in v0.4.0.
	IngressTLSEnabled bool
}

type entry struct {
	snapshot atomic.Value // Snapshot
	history  *History
	cancel   context.CancelFunc
	done     chan struct{}
	specHash string
}

// New constructs a Manager. log defaults to slog.Default() if nil.
func New(rt runtime.Runtime, log *slog.Logger) *Manager {
	return NewWithOptions(rt, log, Options{})
}

// NewWithOptions is like New but accepts an Options struct. Used by
// the CLI to plumb the ingress HTTP port for probe-via-ingress.
func NewWithOptions(rt runtime.Runtime, log *slog.Logger, opts Options) *Manager {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		rt:                rt,
		log:               log,
		ingressHTTPPort:   opts.IngressHTTPPort,
		ingressHTTPSPort:  opts.IngressHTTPSPort,
		ingressTLSEnabled: opts.IngressTLSEnabled,
		entries:           make(map[string]*entry),
		mgrCtx:            ctx,
		mgrCancel:         cancel,
	}
}

// Run blocks until ctx cancels, then signals every per-replica goroutine
// to stop and waits for them to exit.
func (m *Manager) Run(ctx context.Context) {
	<-ctx.Done()
	m.mgrCancel()
	m.wg.Wait()
}

// Track starts probing the given container. Idempotent: re-tracking
// with the same effective probe spec is a no-op; a different spec
// cancels and restarts the goroutine.
//
// An empty spec.Health (no path AND no command) is treated as a
// trust-the-runtime-state mode — the snapshot reports HealthOK=true
// and no goroutine is started.
func (m *Manager) Track(containerID string, spec types.TaskDef) error {
	if containerID == "" {
		return errors.New("probe/manager: Track requires non-empty containerID")
	}
	h := hashSpec(spec)

	m.mu.Lock()
	if cur, ok := m.entries[containerID]; ok && cur.specHash == h {
		m.mu.Unlock()
		return nil
	}
	if cur, ok := m.entries[containerID]; ok {
		// Spec changed — cancel old goroutine and replace.
		if cur.cancel != nil {
			cur.cancel()
		}
		delete(m.entries, containerID)
		m.mu.Unlock()
		if cur.done != nil {
			<-cur.done
		}
		m.mu.Lock()
	}

	e := &entry{
		history:  &History{},
		specHash: h,
	}

	if isEmptyHealth(spec.Health) {
		e.snapshot.Store(Snapshot{ContainerID: containerID, HealthOK: true})
		m.entries[containerID] = e
		m.mu.Unlock()
		return nil
	}

	// Start a fresh goroutine; ctx is a child of the manager's master ctx.
	ctx, cancel := context.WithCancel(m.mgrCtx)
	e.cancel = cancel
	e.done = make(chan struct{})
	e.snapshot.Store(Snapshot{ContainerID: containerID, HealthOK: false})
	m.entries[containerID] = e
	m.mu.Unlock()

	m.wg.Go(func() { m.probeLoop(ctx, containerID, spec, e) })
	return nil
}

// Untrack stops probing the given container. Idempotent.
func (m *Manager) Untrack(containerID string) {
	m.mu.Lock()
	cur, ok := m.entries[containerID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.entries, containerID)
	m.mu.Unlock()

	if cur.cancel != nil {
		cur.cancel()
	}
	if cur.done != nil {
		<-cur.done
	}
}

// TrackedIDs returns the container IDs currently tracked by the Manager.
// Returned slice is a snapshot; safe to iterate without holding any lock.
func (m *Manager) TrackedIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.entries))
	for id := range m.entries {
		out = append(out, id)
	}
	return out
}

// Snapshot returns the latest health snapshot for the container, or
// (zero, false) if untracked.
func (m *Manager) Snapshot(containerID string) (Snapshot, bool) {
	m.mu.Lock()
	e, ok := m.entries[containerID]
	m.mu.Unlock()
	if !ok {
		return Snapshot{}, false
	}
	v := e.snapshot.Load()
	if v == nil {
		return Snapshot{ContainerID: containerID}, true
	}
	return v.(Snapshot), true
}

// probeLoop runs one container's probe goroutine. Exits on ctx cancel.
// Scheduled via sync.WaitGroup.Go (Go 1.25) — no manual Add/Done pair.
func (m *Manager) probeLoop(ctx context.Context, id string, spec types.TaskDef, e *entry) {
	defer close(e.done)

	interval := spec.Health.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	timeout := spec.Health.Timeout
	if timeout <= 0 || timeout > interval {
		timeout = interval / 2
		if timeout <= 0 {
			timeout = time.Second
		}
	}
	retries := spec.Health.Retries
	if retries <= 0 {
		retries = 3
	}

	var httpProbe *HTTPProbe
	var execProbe *ExecProbe
	if spec.Health.Path != "" {
		// Probe-via-ingress path: dial 127.0.0.1:<ingress-http-port>
		// and inject the route's host header. Works on macOS Docker
		// Desktop where the bridge subnet is unreachable from the host.
		if spec.Health.Via == "ingress" && m.ingressHTTPPort > 0 {
			host := lookupRouteHost(spec)
			if host == "" {
				m.log.Error("probe: Health.Via=ingress but no [[route]] declared", "container", id)
				return
			}
			ing := IngressInfo{
				HTTPPort:   m.ingressHTTPPort,
				HTTPSPort:  m.ingressHTTPSPort,
				TLSEnabled: m.ingressTLSEnabled,
			}
			httpProbe = NewHTTPProbeViaIngress(ing, host, spec.Health.Path, timeout, spec.Health.FollowRedirects)
		} else {
			info, err := m.rt.InspectContainer(ctx, id)
			if err != nil {
				m.log.Error("probe: inspect for HTTP probe failed", "container", id, "err", err)
				return
			}
			port := spec.Health.Port
			if port == 0 && len(spec.Expose) > 0 {
				port = spec.Expose[0].Container
			}
			httpProbe = NewHTTPProbe(info.IPAddress, port, spec.Health.Path, timeout, spec.Health.FollowRedirects)
		}
	}
	if len(spec.Health.Command) > 0 {
		execProbe = NewExecProbe(m.rt, id, spec.Health.Command, timeout)
	}

	runOnce := func() {
		var r Result
		if httpProbe != nil {
			r = httpProbe.Run(ctx)
			if !r.Healthy {
				m.recordResult(id, e, r, retries)
				return
			}
		}
		if execProbe != nil {
			r = execProbe.Run(ctx)
		}
		m.recordResult(id, e, r, retries)
	}

	runOnce() // first probe fires immediately

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

// recordResult updates history + snapshot and emits slog transitions.
func (m *Manager) recordResult(id string, e *entry, r Result, retries int) {
	e.history.Append(r)
	streak := e.history.Streak()

	prev := e.snapshot.Load().(Snapshot)
	next := Snapshot{
		ContainerID: id,
		HealthOK:    r.Healthy,
		LastProbeAt: r.At,
		LastErr:     e.history.LastErr(),
		Streak:      streak,
	}
	e.snapshot.Store(next)

	switch {
	case prev.HealthOK != next.HealthOK:
		m.log.Info("probe: health transition", "container", id, "healthy", next.HealthOK, "streak", streak)
	case streak == retries:
		m.log.Warn("probe: streak reached retries", "container", id, "streak", streak, "retries", retries)
	default:
		m.log.Debug("probe: result", "container", id, "healthy", next.HealthOK, "latency_ms", r.Latency.Milliseconds())
	}
}

// lookupRouteHost returns the first L7 [[route]].host on the spec, or
// "" when none exist. Used by the probe-via-ingress path.
func lookupRouteHost(spec types.TaskDef) string {
	for _, r := range spec.Routes {
		if r.L4 == "" && r.Host != "" {
			return r.Host
		}
	}
	return ""
}

// isEmptyHealth reports whether the [health] block is the zero value.
func isEmptyHealth(h types.HealthCheck) bool {
	return h.Path == "" && len(h.Command) == 0
}

// hashSpec returns a stable digest of the probe-relevant subset of the
// TaskDef so Track can detect spec drift cheaply.
func hashSpec(spec types.TaskDef) string {
	h := sha256.New()
	fmt.Fprintf(h, "p=%s|port=%d|cmd=%v|iv=%s|to=%s|rt=%d|exposeN=%d",
		spec.Health.Path, spec.Health.Port, spec.Health.Command,
		spec.Health.Interval, spec.Health.Timeout, spec.Health.Retries,
		len(spec.Expose))
	if len(spec.Expose) > 0 {
		fmt.Fprintf(h, "|e0=%d", spec.Expose[0].Container)
	}
	return hex.EncodeToString(h.Sum(nil))
}
