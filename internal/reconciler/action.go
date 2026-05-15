package reconciler

import (
	"context"
	"fmt"

	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
)

// Apply executes one Action against the given Runtime. Errors are
// wrapped with the action context so loop logging is informative.
//
// Strategy: naive remove-then-create per replica (Complexity Tracking
// deviation #2). Real start-first/stop-first arrives in Feature 002.
func Apply(ctx context.Context, runtime rt.Runtime, a Action) error {
	switch a.Type {
	case ActionCreate:
		return applyCreate(ctx, runtime, a)
	case ActionRemove:
		return runtime.RemoveContainer(ctx, a.ContainerID, true)
	case ActionReplace:
		// Naive: remove the stale, then create the fresh.
		if err := runtime.RemoveContainer(ctx, a.ContainerID, true); err != nil {
			return fmt.Errorf("reconciler: replace[remove %s]: %w", a.ContainerID, err)
		}
		return applyCreate(ctx, runtime, a)
	default:
		return fmt.Errorf("reconciler: unknown action type %q", a.Type)
	}
}

func applyCreate(ctx context.Context, runtime rt.Runtime, a Action) error {
	if err := runtime.PullImage(ctx, a.Spec.Image); err != nil {
		return fmt.Errorf("reconciler: pull %s: %w", a.Spec.Image, err)
	}
	spec := rt.ContainerSpec{
		Name:      dockerlabels.ContainerNameFor(a.Project, a.Service, a.Replica),
		Image:     a.Spec.Image,
		Env:       a.Spec.Env,
		Volumes:   a.Spec.Volumes,
		Ports:     a.Spec.Expose,
		Security:  a.Spec.Security,
		Resources: a.Spec.Resources,
		Labels: map[string]string{
			dockerlabels.LabelProject:  a.Project,
			dockerlabels.LabelService:  a.Service,
			dockerlabels.LabelReplica:  fmt.Sprintf("%d", a.Replica),
			dockerlabels.LabelSpecHash: a.SpecHash,
		},
	}
	id, err := runtime.CreateContainer(ctx, spec)
	if err != nil {
		return fmt.Errorf("reconciler: create %s: %w", spec.Name, err)
	}
	if err := runtime.StartContainer(ctx, id); err != nil {
		return fmt.Errorf("reconciler: start %s: %w", id, err)
	}
	return nil
}
