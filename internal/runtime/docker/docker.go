package docker

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/docker/client"
)

// Runtime is the Docker-backed implementation of
// [github.com/proxa-server/proxa/internal/runtime.Runtime].
type Runtime struct {
	cli    dockerClient
	nodeID string
}

// dockerClient is the minimum surface this package needs from
// docker/docker/client. Defined as a private interface so mocks can
// substitute it in tests without depending on the real Docker daemon.
type dockerClient interface {
	containerClient
	imageClient
	systemClient
	execClient
	logsClient
	closer
}

type closer interface {
	Close() error
}

// New returns a Runtime connected to the local Docker daemon via the
// usual env conventions (DOCKER_HOST, default Unix socket otherwise).
// nodeID is stamped into the proxa.node label on every container.
func New(ctx context.Context, nodeID string) (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: connect: %w", err)
	}
	if nodeID == "" {
		nodeID = "node-local"
	}
	return &Runtime{cli: cli, nodeID: nodeID}, nil
}

// Name returns "docker".
func (r *Runtime) Name() string { return "docker" }

// Version returns the daemon version string.
func (r *Runtime) Version(ctx context.Context) (string, error) {
	v, err := r.cli.ServerVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("runtime/docker: server version: %w", err)
	}
	return v.Version, nil
}

// Close releases the underlying Docker client.
func (r *Runtime) Close() error {
	return r.cli.Close()
}

// ErrNotImplemented preserved for API-shape compatibility with the
// noop runtime stub from feature 000.
var ErrNotImplemented = errors.New("runtime/docker: not implemented")
