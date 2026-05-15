// Package runtime is the abstraction over a container runtime — Docker
// in v0.x, containerd or OCI direct in later versions. Every container
// operation in Proxa goes through [Runtime]; schedulers, reconcilers,
// and CLI commands never import a runtime SDK directly.
//
// See specs/000-foundation/contracts/runtime.md for the full behavioral
// contract.
package runtime

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/proxa-server/proxa/pkg/types"
)

// Runtime is the contract every container backend implements.
// Implementations live in this package: dockerRuntime (v0.x),
// containerdRuntime (v0.x late or v1.0).
//
// Behavioral rules (full contract in
// specs/000-foundation/contracts/runtime.md):
//
//   - Every method takes context.Context as first parameter.
//   - ListContainers requires a non-empty Project filter.
//   - CreateContainer MUST call security.Apply on spec.Security before
//     submitting to the backend; constitution §II.
//   - Errors wrap with fmt.Errorf("runtime/<impl>: %w", err).
type Runtime interface {
	Name() string
	Version(ctx context.Context) (string, error)

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

	// Exec for health checks and `proxa exec`.
	Exec(ctx context.Context, id string, cmd []string, opts ExecOpts) (*ExecResult, error)
}

// ContainerSpec is the runtime-agnostic description of one container
// to create. The Name field is set by the reconciler, project-prefixed.
type ContainerSpec struct {
	Name      string
	Image     string
	Env       map[string]string
	Cmd       []string
	Volumes   []types.VolumeMount
	Ports     []types.PortSpec
	Security  types.SecurityProfile
	Resources types.ResourceLimits
	Labels    map[string]string // proxa.project, proxa.service, ...
}

// PortSpecAlias is a re-export for sub-packages that build host
// configs without needing to import pkg/types directly.
type PortSpecAlias = types.PortSpec

// ImageInfo describes an image known to the runtime.
type ImageInfo struct {
	Ref     string
	Digest  string
	Size    int64
	Created time.Time
}

// ContainerInfo describes a container known to the runtime.
type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	State  string // created | running | exited | paused | ...
	Health string // healthy | unhealthy | starting | none
	Labels map[string]string
}

// ListFilter scopes a ListContainers call. Project is required.
type ListFilter struct {
	Project string
	Labels  map[string]string
}

// LogOpts controls a StreamLogs call.
type LogOpts struct {
	Follow     bool
	Tail       int // -1 = all
	Timestamps bool
	Since      time.Time
}

// ContainerStats is one snapshot of container resource usage.
type ContainerStats struct {
	CPUPercent  float64
	MemoryBytes int64
	MemoryLimit int64
	NetRxBytes  int64
	NetTxBytes  int64
}

// ExecOpts controls an Exec call.
type ExecOpts struct {
	Stdin   io.Reader
	TTY     bool
	Timeout time.Duration
}

// ExecResult is the outcome of an Exec call.
type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// ErrNotImplemented is returned by stub implementations of [Runtime].
var ErrNotImplemented = errors.New("runtime: not implemented")

// noopRuntime is a stub that satisfies [Runtime] by returning
// ErrNotImplemented for every method. Used by unit tests in later
// features that need to inject a Runtime without touching a real backend.
type noopRuntime struct{}

// Compile-time assertion that noopRuntime satisfies Runtime; also
// keeps the methods reachable for staticcheck.
var _ Runtime = noopRuntime{}

func (noopRuntime) Name() string                                              { return "noop" }
func (noopRuntime) Version(context.Context) (string, error)                   { return "", ErrNotImplemented }
func (noopRuntime) PullImage(context.Context, string) error                   { return ErrNotImplemented }
func (noopRuntime) InspectImage(context.Context, string) (*ImageInfo, error)  { return nil, ErrNotImplemented }
func (noopRuntime) CreateContainer(context.Context, ContainerSpec) (string, error) {
	return "", ErrNotImplemented
}
func (noopRuntime) StartContainer(context.Context, string) error                       { return ErrNotImplemented }
func (noopRuntime) StopContainer(context.Context, string, time.Duration) error         { return ErrNotImplemented }
func (noopRuntime) RemoveContainer(context.Context, string, bool) error                { return ErrNotImplemented }
func (noopRuntime) InspectContainer(context.Context, string) (*ContainerInfo, error)   { return nil, ErrNotImplemented }
func (noopRuntime) ListContainers(context.Context, ListFilter) ([]ContainerInfo, error) {
	return nil, ErrNotImplemented
}
func (noopRuntime) StreamLogs(context.Context, string, LogOpts) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}
func (noopRuntime) Stats(context.Context, string) (*ContainerStats, error) { return nil, ErrNotImplemented }
func (noopRuntime) Exec(context.Context, string, []string, ExecOpts) (*ExecResult, error) {
	return nil, ErrNotImplemented
}
