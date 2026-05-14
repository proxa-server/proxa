package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/system"

	"github.com/proxa-server/proxa/internal/runtime"
)

// systemClient is the subset for daemon-wide info (used by Version()).
type systemClient interface {
	ServerVersion(ctx context.Context) (types.Version, error)
	Info(ctx context.Context) (system.Info, error)
}

// Exec, Stats, and StreamLogs are intentionally minimal stubs in v0.0.
// The reconciler doesn't need them to satisfy SC-001 through SC-007;
// they're declared on the Runtime interface to honor the contract from
// 000-foundation but their full implementations land with Feature 002
// (health checks need Stats; the dashboard log viewer needs StreamLogs).

// Exec is unimplemented in v0.0.
func (r *Runtime) Exec(ctx context.Context, id string, cmd []string, opts runtime.ExecOpts) (*runtime.ExecResult, error) {
	return nil, ErrNotImplemented
}

// Stats is unimplemented in v0.0.
func (r *Runtime) Stats(ctx context.Context, id string) (*runtime.ContainerStats, error) {
	return nil, ErrNotImplemented
}

// StreamLogs is unimplemented in v0.0.
func (r *Runtime) StreamLogs(ctx context.Context, id string, opts runtime.LogOpts) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}
