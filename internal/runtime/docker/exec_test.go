package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/proxa-server/proxa/internal/runtime"
)

func TestExecSuccess(t *testing.T) {
	m := &mockDockerClient{
		execAttachStdout: "hello\n",
		execAttachStderr: "",
		execInspectExit:  0,
	}
	r := runtimeWithMock(m)

	out, err := r.Exec(context.Background(), "c1", []string{"echo", "hello"}, runtime.ExecOpts{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", out.ExitCode)
	}
	if string(out.Stdout) != "hello\n" {
		t.Errorf("Stdout = %q, want 'hello\\n'", out.Stdout)
	}
	if len(m.execCreateCalls) != 1 {
		t.Fatalf("expected 1 ContainerExecCreate call, got %d", len(m.execCreateCalls))
	}
	got := m.execCreateCalls[0]
	if got.containerID != "c1" {
		t.Errorf("containerID = %q", got.containerID)
	}
	if !got.opts.AttachStdout || !got.opts.AttachStderr {
		t.Errorf("AttachStdout/Stderr should be true")
	}
}

func TestExecNonZeroExit(t *testing.T) {
	m := &mockDockerClient{
		execAttachStdout: "",
		execAttachStderr: "boom\n",
		execInspectExit:  17,
	}
	r := runtimeWithMock(m)

	out, err := r.Exec(context.Background(), "c1", []string{"false"}, runtime.ExecOpts{})
	if err != nil {
		t.Fatalf("Exec returned err on non-zero exit: %v", err)
	}
	if out.ExitCode != 17 {
		t.Errorf("ExitCode = %d, want 17", out.ExitCode)
	}
	if string(out.Stderr) != "boom\n" {
		t.Errorf("Stderr = %q", out.Stderr)
	}
}

func TestExecAttachError(t *testing.T) {
	m := &mockDockerClient{
		execAttachErr: errors.New("conn closed"),
	}
	r := runtimeWithMock(m)

	_, err := r.Exec(context.Background(), "c1", []string{"true"}, runtime.ExecOpts{})
	if err == nil {
		t.Fatalf("expected attach error")
	}
}

func TestExecInspectError(t *testing.T) {
	m := &mockDockerClient{
		execInspectErr: errors.New("no such exec"),
	}
	r := runtimeWithMock(m)

	_, err := r.Exec(context.Background(), "c1", []string{"true"}, runtime.ExecOpts{})
	if err == nil {
		t.Fatalf("expected inspect error")
	}
}

func TestExecRejectsEmptyArgs(t *testing.T) {
	r := runtimeWithMock(&mockDockerClient{})
	if _, err := r.Exec(context.Background(), "", []string{"x"}, runtime.ExecOpts{}); err == nil {
		t.Errorf("empty container id should fail")
	}
	if _, err := r.Exec(context.Background(), "c1", nil, runtime.ExecOpts{}); err == nil {
		t.Errorf("empty cmd should fail")
	}
}
