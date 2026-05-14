# Contract: `internal/store.StateStore`

Persistence for all control-plane state. SQLite-backed in v0.x; etcd-backed in v1.0. Handlers and reconcilers depend on this interface, never on a database driver.

## Go signature (v0.0)

```go
package store

import (
    "context"
    "errors"
    "time"

    "github.com/proxa-server/proxa/pkg/types"
)

// StateStore is the abstract control-plane data store.
// Implementations: sqliteStore (v0.x), etcdStore (v1.0).
type StateStore interface {
    // Lifecycle
    Open(ctx context.Context, dsn string) error
    Close() error
    Migrate(ctx context.Context) error // idempotent schema setup

    // Projects (constitution §III)
    CreateProject(ctx context.Context, p types.Project) error
    GetProject(ctx context.Context, name string) (*types.Project, error)
    ListProjects(ctx context.Context) ([]types.Project, error)
    DeleteProject(ctx context.Context, name string) error

    // Services (project-scoped)
    PutService(ctx context.Context, project string, svc types.Service) error
    GetService(ctx context.Context, project, name string) (*types.Service, error)
    ListServices(ctx context.Context, project string) ([]types.Service, error)
    DeleteService(ctx context.Context, project, name string) error
    WatchServices(ctx context.Context, project string) (<-chan ServiceEvent, error)

    // Jobs (project-scoped)
    PutJob(ctx context.Context, project string, j types.Job) error
    GetJob(ctx context.Context, project, name string) (*types.Job, error)
    ListJobs(ctx context.Context, project string) ([]types.Job, error)
    DeleteJob(ctx context.Context, project, name string) error

    // Nodes (cluster-wide; not project-scoped)
    PutNode(ctx context.Context, n types.Node) error
    GetNode(ctx context.Context, id string) (*types.Node, error)
    ListNodes(ctx context.Context) ([]types.Node, error)
    DeleteNode(ctx context.Context, id string) error

    // Heartbeats (separate path to avoid contention with full PutNode)
    Heartbeat(ctx context.Context, nodeID string, at time.Time) error

    // Policies + Subjects (RBAC)
    PutSubject(ctx context.Context, s types.Subject) error
    GetSubject(ctx context.Context, id string) (*types.Subject, error)
    PutPolicy(ctx context.Context, p types.Policy) error
    ListPoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error)
    DeletePolicy(ctx context.Context, id string) error

    // Generic transactions for compound operations (deploy, rollback)
    Tx(ctx context.Context, fn func(Tx) error) error
}

type Tx interface {
    // Mirrors the read/write methods above; commits on fn return nil.
    // Implementations free to add type-specific helpers.
}

type ServiceEvent struct {
    Type    string         // created | updated | deleted
    Project string
    Name    string
    Service *types.Service // nil for deleted
}

var (
    ErrNotFound       = errors.New("store: not found")
    ErrAlreadyExists  = errors.New("store: already exists")
    ErrNotImplemented = errors.New("store: not implemented")
)
```

## Behavioral contract

1. **`Migrate` is idempotent.** Repeated calls are safe. Schema version tracked internally.
2. **Project scoping is enforced at the interface boundary.** Every service/job/secret operation accepts `project` explicitly. An empty `project` is an error (`ErrInvalid`).
3. **`WatchServices` is best-effort.** Implementations may drop intermediate events under load; consumers MUST reconcile against `ListServices` after any reconnect. SQLite impl uses polling; etcd impl uses native watches.
4. **`Heartbeat` is high-frequency.** Implementations should not run a full row update; an indexed `last_heartbeat` column update with no other side effects is fine.
5. **`Tx` is atomic.** A returned error rolls back. Implementations are not required to support nested transactions.
6. **All errors wrap with `fmt.Errorf("store/<impl>: %w", err)`.**

## Foundation deliverable

`internal/store/store.go` declares the interface + sentinel errors. **No implementation** beyond a stub `noopStore` (returns `ErrNotImplemented`). SQLite implementation arrives in `001-core-loop` or shortly after.
