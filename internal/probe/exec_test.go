package probe

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
)

// fakeRuntime is a tiny in-package Runtime stub for Exec testing.
// Returns configured ExitCode / err for each Exec call.
type fakeRuntime struct {
	exitCode int
	err      error
}

func (fakeRuntime) Name() string                                      { return "fake" }
func (fakeRuntime) Version(context.Context) (string, error)           { return "test", nil }
func (fakeRuntime) PullImage(context.Context, string) error           { return nil }
func (fakeRuntime) InspectImage(context.Context, string) (*runtime.ImageInfo, error) {
	return nil, errors.New("not implemented")
}
func (fakeRuntime) CreateContainer(context.Context, runtime.ContainerSpec) (string, error) {
	return "", nil
}
func (fakeRuntime) StartContainer(context.Context, string) error                       { return nil }
func (fakeRuntime) StopContainer(context.Context, string, time.Duration) error         { return nil }
func (fakeRuntime) RemoveContainer(context.Context, string, bool) error                { return nil }
func (fakeRuntime) InspectContainer(context.Context, string) (*runtime.ContainerInfo, error) {
	return nil, errors.New("not implemented")
}
func (fakeRuntime) ListContainers(context.Context, runtime.ListFilter) ([]runtime.ContainerInfo, error) {
	return nil, nil
}
func (fakeRuntime) StreamLogs(context.Context, string, runtime.LogOpts) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (fakeRuntime) Stats(context.Context, string) (*runtime.ContainerStats, error) {
	return nil, errors.New("not implemented")
}
func (f fakeRuntime) Exec(context.Context, string, []string, runtime.ExecOpts) (*runtime.ExecResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &runtime.ExecResult{ExitCode: f.exitCode}, nil
}

func TestExecProbeHealthy(t *testing.T) {
	p := NewExecProbe(fakeRuntime{exitCode: 0}, "c1", []string{"true"}, time.Second)
	r := p.Run(context.Background())
	if !r.Healthy {
		t.Errorf("expected Healthy on exit 0, got err=%v", r.Err)
	}
}

func TestExecProbeUnhealthy(t *testing.T) {
	p := NewExecProbe(fakeRuntime{exitCode: 1}, "c1", []string{"false"}, time.Second)
	r := p.Run(context.Background())
	if r.Healthy {
		t.Errorf("expected Healthy=false on non-zero exit")
	}
	if r.Err == nil {
		t.Errorf("expected Err on non-zero exit")
	}
}

func TestExecProbeRuntimeError(t *testing.T) {
	p := NewExecProbe(fakeRuntime{err: errors.New("no such container")}, "c1", []string{"sh"}, time.Second)
	r := p.Run(context.Background())
	if r.Healthy {
		t.Errorf("expected Healthy=false when Runtime errors")
	}
	if r.Err == nil {
		t.Errorf("expected Err on Runtime error")
	}
}
