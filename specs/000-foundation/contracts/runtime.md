# Contract: `internal/runtime.Runtime`

The abstraction over a container runtime (Docker in v0.x; containerd or OCI direct later). Every container-affecting operation in Proxa goes through this interface — schedulers, reconcilers, and CLI commands never import a runtime SDK directly.

## Go signature (v0.0)

```go
package runtime

import (
    "context"
    "io"
    "time"

    "github.com/proxa-server/proxa/pkg/types"
)

// Runtime is the contract every container backend implements.
// Implementations: dockerRuntime (v0.x), containerdRuntime (v0.x late or v1.0).
type Runtime interface {
    // Identity
    Name() string                                // "docker", "containerd"
    Version(ctx context.Context) (string, error) // runtime version string

    // Image lifecycle
    PullImage(ctx context.Context, ref string) error
    InspectImage(ctx context.Context, ref string) (*ImageInfo, error)

    // Container lifecycle
    CreateContainer(ctx context.Context, spec ContainerSpec) (id string, err error)
    StartContainer(ctx context.Context, id string) error
    StopContainer(ctx context.Context, id string, gracePeriod time.Duration) error
    RemoveContainer(ctx context.Context, id string, force bool) error
    InspectContainer(ctx context.Context, id string) (*ContainerInfo, error)
    ListContainers(ctx context.Context, filter ListFilter) ([]ContainerInfo, error)

    // Observability
    StreamLogs(ctx context.Context, id string, opts LogOpts) (io.ReadCloser, error)
    Stats(ctx context.Context, id string) (*ContainerStats, error)

    // Exec (for health checks + user `proxa exec`)
    Exec(ctx context.Context, id string, cmd []string, opts ExecOpts) (*ExecResult, error)
}

type ContainerSpec struct {
    Name     string                  // proxa-managed; project-prefixed
    Image    string
    Env      map[string]string
    Cmd      []string
    Volumes  []types.VolumeMount
    Ports    []types.PortSpec
    Security types.SecurityProfile   // see security.md
    Resources types.ResourceLimits
    Labels   map[string]string       // proxa.project, proxa.service, etc.
}

type ImageInfo struct {
    Ref     string
    Digest  string
    Size    int64
    Created time.Time
}

type ContainerInfo struct {
    ID     string
    Name   string
    Image  string
    State  string // created|running|exited|paused|...
    Health string // healthy|unhealthy|starting|none
    Labels map[string]string
}

type ListFilter struct {
    Project   string            // required; "" = error
    Labels    map[string]string // additional filters
}

type LogOpts struct {
    Follow     bool
    Tail       int // -1 = all
    Timestamps bool
    Since      time.Time
}

type ContainerStats struct {
    CPUPercent  float64
    MemoryBytes int64
    MemoryLimit int64
    NetRxBytes  int64
    NetTxBytes  int64
}

type ExecOpts struct {
    Stdin    io.Reader
    TTY      bool
    Timeout  time.Duration
}

type ExecResult struct {
    ExitCode int
    Stdout   []byte
    Stderr   []byte
}

// ErrNotImplemented is returned by stub implementations.
var ErrNotImplemented = errors.New("runtime: not implemented")
```

## Behavioral contract

1. **Every method takes `context.Context` as first param** (§IV). Cancellation aborts in-flight work; deadline propagates.
2. **`ListContainers` requires a project filter.** Empty `Project` returns an error. Constitution §III: no global flat namespace, ever.
3. **`CreateContainer` MUST set `Security` defaults if zero-valued**: `CapDrop: ["ALL"]`, `NoNewPrivileges: true`, `User != "root"`. The runtime *enforces* §II; callers cannot bypass.
4. **`StopContainer` honors `gracePeriod`** before SIGKILL. Default if zero: 10s.
5. **Logs and stats are streaming.** Caller closes the `ReadCloser` to terminate the stream.
6. **Errors wrap with `fmt.Errorf("runtime/%s: %w", impl.Name(), err)`** so the impl is identifiable in logs.

## Foundation deliverable

`internal/runtime/runtime.go` declares the interface + types above. **No concrete implementation.** A stub may exist as `noopRuntime` returning `ErrNotImplemented` for all methods, used for tests in later features.
