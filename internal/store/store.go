// Package store is the abstract control-plane data store. SQLite-backed
// in v0.x, etcd-backed in v1.0. Handlers and reconcilers depend on the
// [StateStore] interface, never on a database driver.
//
// See specs/000-foundation/contracts/statestore.md for the full
// behavioral contract.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/proxa-server/proxa/pkg/types"
)

// StateStore is the abstract control-plane data store.
// Implementations live in this package: sqliteStore (v0.x),
// etcdStore (v1.0).
//
// Behavioral rules (full contract in
// specs/000-foundation/contracts/statestore.md):
//
//   - Migrate is idempotent.
//   - Project scoping is enforced at the interface boundary; an empty
//     project for project-scoped methods is an error.
//   - WatchServices is best-effort; consumers reconcile against
//     ListServices after any reconnect.
//   - Tx is atomic; nested transactions are not required.
type StateStore interface {
	Open(ctx context.Context, dsn string) error
	Close() error
	Migrate(ctx context.Context) error

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

	// Nodes (cluster-wide)
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

	// Generic transactions for compound operations (deploy, rollback).
	Tx(ctx context.Context, fn func(Tx) error) error
}

// Tx is the transaction handle passed to [StateStore.Tx]'s callback.
// Implementations MAY add type-specific helpers; the minimum surface
// mirrors StateStore's read/write methods.
type Tx interface{}

// ServiceEvent is one notification from [StateStore.WatchServices].
type ServiceEvent struct {
	Type    string // created | updated | deleted
	Project string
	Name    string
	Service *types.Service // nil for deleted
}

var (
	// ErrNotFound is returned when the requested entity does not exist.
	ErrNotFound = errors.New("store: not found")

	// ErrAlreadyExists is returned when creating an entity whose unique
	// key collides with an existing row.
	ErrAlreadyExists = errors.New("store: already exists")

	// ErrNotImplemented is returned by stub implementations.
	ErrNotImplemented = errors.New("store: not implemented")
)

// noopStore satisfies [StateStore] with ErrNotImplemented for every
// method. Useful as a placeholder in unit tests.
type noopStore struct{}

// Compile-time assertion that noopStore satisfies StateStore.
var _ StateStore = noopStore{}

func (noopStore) Open(context.Context, string) error      { return ErrNotImplemented }
func (noopStore) Close() error                            { return ErrNotImplemented }
func (noopStore) Migrate(context.Context) error           { return ErrNotImplemented }
func (noopStore) CreateProject(context.Context, types.Project) error { return ErrNotImplemented }
func (noopStore) GetProject(context.Context, string) (*types.Project, error) {
	return nil, ErrNotImplemented
}
func (noopStore) ListProjects(context.Context) ([]types.Project, error) { return nil, ErrNotImplemented }
func (noopStore) DeleteProject(context.Context, string) error           { return ErrNotImplemented }
func (noopStore) PutService(context.Context, string, types.Service) error {
	return ErrNotImplemented
}
func (noopStore) GetService(context.Context, string, string) (*types.Service, error) {
	return nil, ErrNotImplemented
}
func (noopStore) ListServices(context.Context, string) ([]types.Service, error) {
	return nil, ErrNotImplemented
}
func (noopStore) DeleteService(context.Context, string, string) error { return ErrNotImplemented }
func (noopStore) WatchServices(context.Context, string) (<-chan ServiceEvent, error) {
	return nil, ErrNotImplemented
}
func (noopStore) PutJob(context.Context, string, types.Job) error { return ErrNotImplemented }
func (noopStore) GetJob(context.Context, string, string) (*types.Job, error) {
	return nil, ErrNotImplemented
}
func (noopStore) ListJobs(context.Context, string) ([]types.Job, error) {
	return nil, ErrNotImplemented
}
func (noopStore) DeleteJob(context.Context, string, string) error            { return ErrNotImplemented }
func (noopStore) PutNode(context.Context, types.Node) error                  { return ErrNotImplemented }
func (noopStore) GetNode(context.Context, string) (*types.Node, error)       { return nil, ErrNotImplemented }
func (noopStore) ListNodes(context.Context) ([]types.Node, error)            { return nil, ErrNotImplemented }
func (noopStore) DeleteNode(context.Context, string) error                   { return ErrNotImplemented }
func (noopStore) Heartbeat(context.Context, string, time.Time) error         { return ErrNotImplemented }
func (noopStore) PutSubject(context.Context, types.Subject) error            { return ErrNotImplemented }
func (noopStore) GetSubject(context.Context, string) (*types.Subject, error) { return nil, ErrNotImplemented }
func (noopStore) PutPolicy(context.Context, types.Policy) error              { return ErrNotImplemented }
func (noopStore) ListPoliciesFor(context.Context, string) ([]types.Policy, error) {
	return nil, ErrNotImplemented
}
func (noopStore) DeletePolicy(context.Context, string) error           { return ErrNotImplemented }
func (noopStore) Tx(context.Context, func(Tx) error) error             { return ErrNotImplemented }
