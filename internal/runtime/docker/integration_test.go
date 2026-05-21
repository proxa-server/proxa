//go:build dockerd

// Integration tests that require a live Docker daemon. Run with:
//
//	go test -tags dockerd -count=1 ./internal/runtime/docker/...
//
// Or via the Makefile target `make test-integration`. Skipped in CI
// by default; enable on a self-hosted runner if/when one exists.

package docker

import (
	"context"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/security"
)

func TestIntegration_RealDockerCreatesSecuredContainer(t *testing.T) {
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

	// Pull a tiny image to keep the test fast.
	const image = "alpine:3.19"
	if err := r.PullImage(ctx, image); err != nil {
		t.Fatalf("pull: %v", err)
	}

	spec := runtime.ContainerSpec{
		Name:  "proxa-test-int-0",
		Image: image,
		Cmd:   []string{"sleep", "3"},
		Labels: map[string]string{
			LabelProject:  "test",
			LabelService:  "int",
			LabelReplica:  "0",
			LabelSpecHash: "sha256:integration",
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

	info, err := r.InspectContainer(ctx, id)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Labels[LabelManaged] != "true" {
		t.Errorf("LabelManaged not stamped on real container: %v", info.Labels)
	}

	// Verify §II defaults landed by listing and finding the container.
	list, err := r.ListContainers(ctx, runtime.ListFilter{Project: "test"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, c := range list {
		if c.ID == id {
			found = true
		}
	}
	if !found {
		t.Errorf("created container not found via ListContainers project filter")
	}
}
