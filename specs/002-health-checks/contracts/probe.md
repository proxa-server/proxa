# Contract: `internal/probe.Probe`

The probe abstraction. Two implementations in v0.2: HTTP and exec. Future TCP / gRPC plug in here.

## Go signature

```go
package probe

import (
    "context"
    "time"
)

// Probe is the contract every probe implementation satisfies.
// One Probe instance per replica.
type Probe interface {
    // Name identifies the probe type ("http", "exec").
    Name() string

    // Run executes one probe attempt. Returns Healthy=true on success;
    // Healthy=false with a non-nil Err on failure. The Latency field
    // is wall-clock time from start to result.
    //
    // The implementation MUST honor ctx cancellation and the
    // configured per-probe timeout.
    Run(ctx context.Context) Result
}

// Result is one probe outcome. See data-model.md for usage.
type Result struct {
    At      time.Time
    Healthy bool
    Latency time.Duration
    Err     error
}
```

## HTTPProbe

```go
type HTTPProbe struct {
    URL     string        // e.g. "http://172.17.0.5:80/healthz"
    Timeout time.Duration // per-attempt timeout
    client  *http.Client  // pre-built; Transport.IdleConnTimeout=15s
}

func NewHTTPProbe(containerIP string, port int, path string, timeout time.Duration) *HTTPProbe
```

Behavior:
- Builds `http.Client` once at construction with a `Transport` that pins `MaxIdleConnsPerHost = 1` and `IdleConnTimeout = 15s` so a removed container doesn't leak its connection.
- `Run(ctx)` issues `GET <URL>` with `ctx` deadline = `min(ctx.Deadline, time.Now+Timeout)`.
- Response status `200..299` → `Healthy: true`.
- Any other status, network error, or timeout → `Healthy: false, Err: <wrapped error>`.
- Body is drained and discarded (so the connection can be reused by `Transport`).

## ExecProbe

```go
type ExecProbe struct {
    Runtime     runtime.Runtime
    ContainerID string
    Cmd         []string
    Timeout     time.Duration
}

func NewExecProbe(rt runtime.Runtime, containerID string, cmd []string, timeout time.Duration) *ExecProbe
```

Behavior:
- `Run(ctx)` calls `rt.Exec(ctx, ContainerID, Cmd, runtime.ExecOpts{Timeout: Timeout})`.
- ExecResult.ExitCode == 0 → `Healthy: true`.
- ExitCode != 0 OR error → `Healthy: false, Err: <wrapped>`.

## Manager

```go
type Manager struct {
    // unexported state — see implementation
}

// New returns a Manager ready to track probes. log is used for slog
// output; runtime is used to construct ExecProbe instances and to
// fetch container IPs for HTTPProbe.
func New(rt runtime.Runtime, log *slog.Logger) *Manager

// Track starts probing the given container. spec.Health drives probe
// selection (path → HTTP, command → exec, both → both must pass).
// If spec.Health is the zero value, no probes run and the container
// is treated as healthy (preserves v0.1.0 behavior).
//
// Idempotent: re-tracking the same containerID with the same spec is
// a no-op. Re-tracking with a different spec restarts the probe loop.
func (m *Manager) Track(containerID string, spec types.TaskDef) error

// Untrack stops probing the given container and cleans up state.
// Idempotent.
func (m *Manager) Untrack(containerID string)

// Snapshot returns the latest health snapshot for a container, or
// (Snapshot{}, false) if the container is not tracked.
func (m *Manager) Snapshot(containerID string) (Snapshot, bool)

// Run blocks until ctx cancels. The Manager's per-replica probe
// goroutines run in the background; Run is here to give the caller
// a single goroutine to wait on for clean shutdown.
func (m *Manager) Run(ctx context.Context)
```

## Behavioral contract

1. **Per-replica goroutines** — one goroutine per `Track`-ed container. Per R-006.
2. **Fixed-interval timing** — probes fire every `interval` from previous probe START; per R-002.
3. **Strict streak** — `retries` consecutive failures → `HealthOK = false`. One success resets streak. Per R-003.
4. **Goroutine lifecycle** — exits cleanly on Untrack OR on the manager's Run ctx cancel. No goroutine leaks (verified by `goleak`-style assertion in tests).
5. **Snapshot is read-only** — concurrent `Snapshot` calls are safe. The returned struct is a copy.
6. **No spec.Health = no probes** — `Track` with empty Health is a no-op; `Snapshot` returns `HealthOK=true` (preserves v0.1.0 behavior per FR-003).
7. **slog logging** —
   - `level=DEBUG` on every probe with `latency_ms`, `result`.
   - `level=INFO` on streak transitions (0 → N or N → 0).
   - `level=WARN` when streak reaches `retries` (replica about to be unhealthy-marked).

## Error handling

The Manager NEVER panics. Probe errors are captured in `Result.Err` and logged but don't crash the goroutine. A misconfigured probe (e.g., HTTPProbe with a malformed URL) would surface as repeated failures → unhealthy → reconciler removes and recreates the container. The next Track call gets the (re-resolved) container IP.
