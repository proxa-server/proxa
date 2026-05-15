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

	mu      sync.Mutex
	entries map[string]*entry

	mgrCtx    context.Context
	mgrCancel context.CancelFunc

	wg sync.WaitGroup
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
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		rt:        rt,
		log:       log,
		entries:   make(map[string]*entry),
		mgrCtx:    ctx,
		mgrCancel: cancel,
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

	m.wg.Add(1)
	go m.probeLoop(ctx, containerID, spec, e)
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
func (m *Manager) probeLoop(ctx context.Context, id string, spec types.TaskDef, e *entry) {
	defer m.wg.Done()
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
		info, err := m.rt.InspectContainer(ctx, id)
		if err != nil {
			m.log.Error("probe: inspect for HTTP probe failed", "container", id, "err", err)
			return
		}
		port := spec.Health.Port
		if port == 0 && len(spec.Expose) > 0 {
			port = spec.Expose[0].Container
		}
		httpProbe = NewHTTPProbe(info.IPAddress, port, spec.Health.Path, timeout)
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
