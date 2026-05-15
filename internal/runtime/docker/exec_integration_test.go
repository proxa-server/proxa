//go:build dockerd
// +build dockerd

package docker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/security"
)

// TestIntegration_Exec verifies the real ContainerExecCreate/Attach/Inspect
// flow against a running Docker daemon. Skipped in CI; runs with
// `go test -tags dockerd ./internal/runtime/docker/...`.
func TestIntegration_Exec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dockerd integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	r, err := New(ctx, "node-test")
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	defer r.Close()

	const image = "alpine:3.19"
	if err := r.PullImage(ctx, image); err != nil {
		t.Fatalf("pull: %v", err)
	}

	spec := runtime.ContainerSpec{
		Name:  "proxa-test-exec-0",
		Image: image,
		Cmd:   []string{"sleep", "30"},
		Labels: map[string]string{
			LabelProject:  "test",
			LabelService:  "exec",
			LabelReplica:  "0",
			LabelSpecHash: "sha256:exec-int",
		},
		Security: security.Default(),
	}

	id, err := r.CreateContainer(ctx, spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = r.RemoveContainer(context.Background(), id, true) })

	if err := r.StartContainer(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Run("exit 0", func(t *testing.T) {
		res, err := r.Exec(ctx, id, []string{"echo", "hello"}, runtime.ExecOpts{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("Exec: %v", err)
		}
		if res.ExitCode != 0 {
			t.Errorf("ExitCode = %d, want 0", res.ExitCode)
		}
		if !strings.Contains(string(res.Stdout), "hello") {
			t.Errorf("Stdout = %q, want to contain 'hello'", res.Stdout)
		}
	})

	t.Run("exit 1", func(t *testing.T) {
		res, err := r.Exec(ctx, id, []string{"sh", "-c", "exit 1"}, runtime.ExecOpts{Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("Exec: %v", err)
		}
		if res.ExitCode != 1 {
			t.Errorf("ExitCode = %d, want 1", res.ExitCode)
		}
	})
}
