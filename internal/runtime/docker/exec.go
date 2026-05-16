package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/proxa-server/proxa/internal/runtime"
)

// systemClient is the subset for daemon-wide info (used by Version()).
type systemClient interface {
	ServerVersion(ctx context.Context) (types.Version, error)
	Info(ctx context.Context) (system.Info, error)
}

// execClient is the subset of docker/docker/client used for exec.
// Kept as a private interface so tests can swap in a mock.
type execClient interface {
	ContainerExecCreate(ctx context.Context, containerID string, options container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, options container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)
}

// Stats and StreamLogs remain stubs in v0.1.x; the dashboard log viewer
// (Feature 004) will replace StreamLogs and metrics (later) will fill Stats.

// Exec runs cmd inside the container and returns its exit code +
// captured stdout/stderr. Honors ctx cancellation. Stdin is not yet
// wired (no probe pathway needs it); pass opts.Stdin to enable later.
func (r *Runtime) Exec(ctx context.Context, id string, cmd []string, opts runtime.ExecOpts) (*runtime.ExecResult, error) {
	if id == "" {
		return nil, errors.New("runtime/docker: exec: container id required")
	}
	if len(cmd) == 0 {
		return nil, errors.New("runtime/docker: exec: cmd required")
	}

	createResp, err := r.cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          opts.TTY,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: exec create: %w", err)
	}

	attachResp, err := r.cli.ContainerExecAttach(ctx, createResp.ID, container.ExecAttachOptions{Tty: opts.TTY})
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: exec attach: %w", err)
	}
	defer attachResp.Close()

	var stdout, stderr bytes.Buffer
	copyErr := make(chan error, 1)
	go func() {
		if opts.TTY {
			// With a TTY the stream is NOT multiplexed.
			_, err := io.Copy(&stdout, attachResp.Reader)
			copyErr <- err
			return
		}
		_, err := stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader)
		copyErr <- err
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-copyErr:
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("runtime/docker: exec copy: %w", err)
		}
	}

	inspect, err := r.cli.ContainerExecInspect(ctx, createResp.ID)
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: exec inspect: %w", err)
	}
	if inspect.Running {
		return nil, errors.New("runtime/docker: exec still running after copy returned")
	}

	return &runtime.ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}, nil
}

// Stats is unimplemented in v0.1.x.
func (r *Runtime) Stats(ctx context.Context, id string) (*runtime.ContainerStats, error) {
	return nil, ErrNotImplemented
}

// StreamLogs is unimplemented in v0.1.x.
func (r *Runtime) StreamLogs(ctx context.Context, id string, opts runtime.LogOpts) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}
