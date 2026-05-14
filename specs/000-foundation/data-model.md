# Phase 1 — Data Model: Shared Types (`pkg/types/`)

Concrete field-level definitions for the seven entities the spec enumerates. These types live in `pkg/types/` (public) so both binaries (`proxa`, `proxa-agent`) and downstream tooling can import them without touching `internal/`.

Conventions across all types:
- JSON struct tags use `lowerCamelCase` for API/dashboard wire format.
- TOML struct tags use `snake_case` to match the user-facing TOML grammar.
- Time fields are `time.Time` in Go; serialized as RFC 3339.
- ID fields are `string` (KSUID generated at creation; opaque to callers).
- `Project string` is mandatory on every resource per constitution §III. Empty string is invalid; `"default"` is the implicit fallback at the API boundary, not in storage.

---

## `TaskDef`

The parsed representation of a user TOML file describing a service or job.

```go
type TaskDef struct {
    Project   string            `toml:"project"   json:"project"`
    Name      string            `toml:"name"      json:"name"`
    Image     string            `toml:"image"     json:"image"`
    Replicas  int               `toml:"replicas"  json:"replicas"`           // services only
    Stateful  bool              `toml:"stateful"  json:"stateful"`           // affects deploy strategy + scheduling
    Security  SecurityProfile   `toml:"security"  json:"security"`           // see internal/security
    Env       map[string]string `toml:"env"       json:"env,omitempty"`
    Volumes   []VolumeMount     `toml:"volumes"   json:"volumes,omitempty"`
    Expose    []PortSpec        `toml:"expose"    json:"expose,omitempty"`
    Strategy  DeployStrategy    `toml:"strategy"  json:"strategy"`           // start-first | stop-first
    Health    HealthCheck       `toml:"health"    json:"health"`
    Resources ResourceLimits    `toml:"resources" json:"resources"`
    Schedule  string            `toml:"schedule"  json:"schedule,omitempty"` // cron expr; jobs only
}
```

### Sub-types (also in `pkg/types/`)

```go
type VolumeMount struct {
    Source   string `toml:"source"   json:"source"`   // host path or named volume
    Target   string `toml:"target"   json:"target"`   // container path
    ReadOnly bool   `toml:"readonly" json:"readOnly"`
}

type PortSpec struct {
    Container int    `toml:"container" json:"container"`
    Host      int    `toml:"host"      json:"host,omitempty"` // 0 = ingress-only
    Protocol  string `toml:"protocol"  json:"protocol"`       // tcp | udp | http | https
}

type DeployStrategy string
const (
    StrategyStartFirst DeployStrategy = "start-first" // stateless default
    StrategyStopFirst  DeployStrategy = "stop-first"  // stateful default
)

type HealthCheck struct {
    Path     string        `toml:"path"     json:"path,omitempty"`     // HTTP probe path
    Port     int           `toml:"port"     json:"port,omitempty"`
    Command  []string      `toml:"command"  json:"command,omitempty"`  // exec probe
    Interval time.Duration `toml:"interval" json:"interval"`
    Timeout  time.Duration `toml:"timeout"  json:"timeout"`
    Retries  int           `toml:"retries"  json:"retries"`
}

type ResourceLimits struct {
    CPU       string `toml:"cpu"       json:"cpu,omitempty"`       // "500m", "2"
    Memory    string `toml:"memory"    json:"memory,omitempty"`    // "256Mi", "2Gi"
    PidsLimit int    `toml:"pidsLimit" json:"pidsLimit,omitempty"`
}
```

**Validation rules** (enforced when TOML parsing lands in 001+):
- `Project`, `Name`, `Image` are required and non-empty.
- `Name` matches `^[a-z0-9][a-z0-9-]{0,62}$`.
- `Replicas >= 0`. `0` means "stopped".
- For a `Job`, `Schedule` is either empty (one-shot) or a valid 5-field cron expression.
- A `Service` (no `Schedule`) must declare a `Health` check.

---

## `Service`

A running stateless or stateful workload tracked in the state store.

```go
type Service struct {
    ID         string             `json:"id"`
    Project    string             `json:"project"`
    Name       string             `json:"name"`
    Spec       TaskDef            `json:"spec"`        // last applied desired state
    Status     ServiceStatus      `json:"status"`
    Replicas   []ReplicaState     `json:"replicas"`
    History    []DeploymentRecord `json:"history"`     // capped; oldest dropped
    CreatedAt  time.Time          `json:"createdAt"`
    UpdatedAt  time.Time          `json:"updatedAt"`
}

type ServiceStatus string
const (
    ServiceStatusPending    ServiceStatus = "pending"
    ServiceStatusReconciling ServiceStatus = "reconciling"
    ServiceStatusHealthy    ServiceStatus = "healthy"
    ServiceStatusDegraded   ServiceStatus = "degraded"
    ServiceStatusFailed     ServiceStatus = "failed"
)

type ReplicaState struct {
    ID          string    `json:"id"`           // container ID
    NodeID      string    `json:"nodeId"`
    Phase       string    `json:"phase"`        // starting|running|exiting|failed
    HealthOK    bool      `json:"healthOk"`
    StartedAt   time.Time `json:"startedAt"`
    LastProbeAt time.Time `json:"lastProbeAt"`
}

type DeploymentRecord struct {
    DeployedAt time.Time `json:"deployedAt"`
    Image      string    `json:"image"`
    Spec       TaskDef   `json:"spec"`         // snapshot for instant rollback
    Outcome    string    `json:"outcome"`      // success | failed | rolled-back
}
```

**State transitions**: `pending → reconciling → (healthy | degraded | failed)`. `degraded → reconciling` when reconciler attempts recovery. `failed` is terminal for a deployment attempt; superseded by next apply.

---

## `Job`

One-shot or cron workload.

```go
type Job struct {
    ID         string    `json:"id"`
    Project    string    `json:"project"`
    Name       string    `json:"name"`
    Spec       TaskDef   `json:"spec"`
    LastRun    *JobRun   `json:"lastRun,omitempty"`
    NextRunAt  time.Time `json:"nextRunAt,omitempty"`
    CreatedAt  time.Time `json:"createdAt"`
    UpdatedAt  time.Time `json:"updatedAt"`
}

type JobRun struct {
    StartedAt time.Time     `json:"startedAt"`
    EndedAt   time.Time     `json:"endedAt,omitempty"`
    Duration  time.Duration `json:"duration"`
    ExitCode  int           `json:"exitCode"`
    Status    string        `json:"status"`    // running | succeeded | failed | timeout
    LogsRef   string        `json:"logsRef"`   // pointer to log storage
}
```

---

## `Node`

Cluster member (server or agent).

```go
type Node struct {
    ID            string        `json:"id"`
    Name          string        `json:"name"`
    Role          NodeRole      `json:"role"`         // server | agent
    Address       string        `json:"address"`      // host:port reachable from control plane
    Resources     NodeResources `json:"resources"`
    ContainerCount int          `json:"containerCount"`
    LastHeartbeat time.Time     `json:"lastHeartbeat"`
    Status        NodeStatus    `json:"status"`
    Labels        map[string]string `json:"labels,omitempty"` // for placement constraints
}

type NodeRole string
const (
    NodeRoleServer NodeRole = "server"
    NodeRoleAgent  NodeRole = "agent"
)

type NodeStatus string
const (
    NodeStatusReady   NodeStatus = "ready"
    NodeStatusDraining NodeStatus = "draining"
    NodeStatusOffline NodeStatus = "offline"
)

type NodeResources struct {
    CPUCores   int   `json:"cpuCores"`
    MemoryMB   int64 `json:"memoryMb"`
    DiskGB     int64 `json:"diskGb"`
}
```

---

## `Project`

A logical grouping. The boundary for RBAC, naming, and quotas (v1.0+).

```go
type Project struct {
    Name      string    `json:"name"`         // primary key; also slug
    CreatedAt time.Time `json:"createdAt"`
}
```

**Validation**: `Name` matches `^[a-z0-9][a-z0-9-]{0,62}$`. `"default"` always exists and is created at first boot.

---

## `Policy`

RBAC binding: subject → role → project scope.

```go
type Policy struct {
    ID        string    `json:"id"`
    SubjectID string    `json:"subjectId"`
    Role      Role      `json:"role"`
    Project   string    `json:"project"`     // "*" = all projects (admin only)
    CreatedAt time.Time `json:"createdAt"`
}

type Role string
const (
    RoleAdmin  Role = "admin"   // all projects, all verbs
    RoleEditor Role = "editor"  // CRUD on services/jobs/secrets in scope
    RoleViewer Role = "viewer"  // read-only in scope
    RoleAgent  Role = "agent"   // per §VI: cluster nodes' identity for control-plane calls
)
```

---

## `Subject`

An authenticated identity (human, OIDC, or service account).

```go
type Subject struct {
    ID       string            `json:"id"`
    Name     string            `json:"name"`
    Email    string            `json:"email,omitempty"`
    Provider string            `json:"provider"`        // local | oidc:<issuer> | token
    Metadata map[string]string `json:"metadata,omitempty"`
}
```

---

## Cross-cutting

- **No global flat namespace.** Every state-store query takes a `project` argument (constitution §III).
- **Compatibility with `StateStore` interface**: Each type is independently JSON-encodable. The SQLite implementation may add per-type columns; the interface contract is "round-trip through JSON without loss."
- **No secrets in these types.** Secret material lives behind `SecretsStore`. `Service.Spec.Env` may reference secrets via `${secret:NAME}` placeholders resolved at container-start time, never persisted decrypted.
