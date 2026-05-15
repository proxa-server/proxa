package reconciler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/proxa-server/proxa/internal/probe"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
)

// Apply executes one Action. Create/Remove go straight to the Runtime;
// Replace routes through the spec-selected Strategy (start-first for
// stateless, stop-first for stateful) so the rollover is probe-gated.
// Errors are wrapped with action context for informative loop logging.
func Apply(ctx context.Context, runtime rt.Runtime, probes *probe.Manager, logger *slog.Logger, a Action) error {
	switch a.Type {
	case ActionCreate:
		return applyCreate(ctx, runtime, a)
	case ActionRemove:
		return runtime.RemoveContainer(ctx, a.ContainerID, true)
	case ActionReplace:
		strategy := SelectStrategy(a.Spec)
		return strategy.Apply(ctx, Request{
			Project:    a.Project,
			Service:    a.Service,
			ReplicaIdx: a.Replica,
			OldID:      a.ContainerID,
			NewSpec:    a.Spec,
			Runtime:    runtime,
			Probes:     probes,
			Logger:     logger,
		})
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
