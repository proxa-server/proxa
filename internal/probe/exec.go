package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
)

// ExecProbe runs a command inside the container via the Runtime's
// Exec method. Exit code 0 = healthy; anything else = unhealthy.
type ExecProbe struct {
	Runtime     runtime.Runtime
	ContainerID string
	Cmd         []string
	Timeout     time.Duration
}

// NewExecProbe constructs an ExecProbe. Timeout defaults to 5s.
func NewExecProbe(rt runtime.Runtime, containerID string, cmd []string, timeout time.Duration) *ExecProbe {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &ExecProbe{
		Runtime:     rt,
		ContainerID: containerID,
		Cmd:         cmd,
		Timeout:     timeout,
	}
}

// Name returns "exec".
func (p *ExecProbe) Name() string { return "exec" }

// Run executes the configured command inside the container.
func (p *ExecProbe) Run(ctx context.Context) Result {
	start := time.Now()
	execCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	res, err := p.Runtime.Exec(execCtx, p.ContainerID, p.Cmd, runtime.ExecOpts{Timeout: p.Timeout})
	if err != nil {
		return Result{At: start, Healthy: false, Latency: time.Since(start),
			Err: fmt.Errorf("probe/exec: %v in %s: %w", p.Cmd, p.ContainerID, err)}
	}
	if res.ExitCode != 0 {
		return Result{At: start, Healthy: false, Latency: time.Since(start),
			Err: fmt.Errorf("probe/exec: %v exited %d", p.Cmd, res.ExitCode)}
	}
	return Result{At: start, Healthy: true, Latency: time.Since(start)}
}
