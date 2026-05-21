//go:build dockerd
// +build dockerd

package docker

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"

	rt "github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/security"
)

// TestIntegration_StreamLogs verifies the real ContainerLogs path
// against a running Docker daemon. Skipped in CI; runs with
// `go test -tags dockerd ./internal/runtime/docker/...`.
func TestIntegration_StreamLogs(t *testing.T) {
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

	spec := rt.ContainerSpec{
		Name:  "proxa-test-logs-0",
		Image: image,
		Cmd:   []string{"sh", "-c", "for i in 1 2 3; do echo line $i; sleep 0.2; done"},
		Labels: map[string]string{
			LabelProject:  "test",
			LabelService:  "logs",
			LabelReplica:  "0",
			LabelSpecHash: "sha256:logs-int",
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

	// Follow the logs and collect the three lines.
	rc, err := r.StreamLogs(ctx, id, rt.LogOpts{Follow: true, Tail: -1})
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	got := []string{}
	deadline := time.Now().Add(5 * time.Second)
	for len(got) < 3 && time.Now().Before(deadline) {
		if scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "line ") {
				got = append(got, line)
			}
		} else if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	if len(got) != 3 {
		t.Fatalf("got %d lines (want 3): %v", len(got), got)
	}
	want := []string{"line 1", "line 2", "line 3"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line[%d] = %q, want %q", i, got[i], w)
		}
	}
}
