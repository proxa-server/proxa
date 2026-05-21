package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/proxa-server/proxa/internal/runtime"
)

// logsClient is the subset of docker/docker/client used for logs.
// Kept private so tests can swap in a mock.
type logsClient interface {
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
}

// StreamLogs opens a log stream for the given container ID. The returned
// ReadCloser delivers stdout AND stderr interleaved (already demuxed via
// stdcopy.StdCopy in a sibling goroutine). The caller copies bytes as-is.
//
// Honors ctx cancellation: cancelling ctx closes the daemon-side reader
// within ~100ms and ends the demux goroutine.
//
// Closing the returned ReadCloser also terminates the demux goroutine
// (the underlying daemon body close propagates an error to StdCopy).
func (r *Runtime) StreamLogs(ctx context.Context, id string, opts runtime.LogOpts) (io.ReadCloser, error) {
	if id == "" {
		return nil, fmt.Errorf("runtime/docker: streamlogs: container id required")
	}

	dockerOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
		Tail:       tailValue(opts.Tail),
	}
	if !opts.Since.IsZero() {
		dockerOpts.Since = opts.Since.UTC().Format("2006-01-02T15:04:05.000000000Z")
	}

	body, err := r.cli.ContainerLogs(ctx, id, dockerOpts)
	if err != nil {
		return nil, fmt.Errorf("runtime/docker: container logs %q: %w", id, err)
	}

	// stdcopy demux happens in a sibling goroutine; we return the pipe
	// reader so the caller sees already-demuxed bytes.
	pr, pw := io.Pipe()
	go func() {
		// StdCopy returns when body returns EOF / ctx cancels / body close.
		_, err := stdcopy.StdCopy(pw, pw, body)
		_ = body.Close()
		_ = pw.CloseWithError(err)
	}()

	return pr, nil
}

// tailValue maps the proxa LogOpts.Tail int to Docker's stringy Tail.
//
//	-1 (or absent) → "all"
//	 0             → "0" (suppresses historical lines; follow-only stream)
//	 N > 0         → "N"
func tailValue(n int) string {
	if n < 0 {
		return "all"
	}
	return fmt.Sprintf("%d", n)
}
