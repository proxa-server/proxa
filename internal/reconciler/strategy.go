package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/proxa-server/proxa/internal/hash"
	"github.com/proxa-server/proxa/internal/probe"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/pkg/types"
)

// Strategy executes a per-replica replacement (old container ID known,
// new spec known) according to the strategy's safety contract. See
// specs/002-health-checks/contracts/strategy.md.
type Strategy interface {
	Name() string
	Apply(ctx context.Context, req Request) error
}

// Request is the per-replacement input to Strategy.Apply.
type Request struct {
	Project    string
	Service    string
	ReplicaIdx int
	OldID      string        // "" when no old container exists (fresh create)
	NewSpec    types.TaskDef // includes Health for probe gating
	Runtime    rt.Runtime
	Probes     *probe.Manager
	Logger     *slog.Logger
}

// ErrRolledBack signals a start-first replacement was reverted because
// the new container failed every probe within the deadline. The old
// container is still running.
var ErrRolledBack = errors.New("reconciler: replacement rolled back")

// SelectStrategy returns the right Strategy for a TaskDef. Honors
// explicit spec.Strategy; falls back to spec.Stateful default.
func SelectStrategy(spec types.TaskDef) Strategy {
	switch spec.Strategy {
	case types.StrategyStartFirst:
		return &StartFirst{}
	case types.StrategyStopFirst:
		return &StopFirst{}
	case "":
		if spec.Stateful {
			return &StopFirst{}
		}
		return &StartFirst{}
	}
	return &StartFirst{} // safe default for unknown values
}

// StartFirst implements the probe-gated rollover for stateless services.
// New container is created and probed BEFORE the old is removed; on
// probe failure the new is torn down and the old is retained.
type StartFirst struct{}

// Name returns "start-first".
func (*StartFirst) Name() string { return string(types.StrategyStartFirst) }

// Apply executes the 9-step start-first contract.
func (*StartFirst) Apply(ctx context.Context, req Request) error {
	canonicalName := dockerlabels.ContainerNameFor(req.Project, req.Service, req.ReplicaIdx)
	tempName := canonicalName + "-new"
	log := req.Logger
	if log == nil {
		log = slog.Default()
	}

	if err := req.Runtime.PullImage(ctx, req.NewSpec.Image); err != nil {
		return fmt.Errorf("start-first: pull %s: %w", req.NewSpec.Image, err)
	}

	specHash := hash.Hash(req.NewSpec)
	newID, err := req.Runtime.CreateContainer(ctx, rt.ContainerSpec{
		Name:      tempName,
		Image:     req.NewSpec.Image,
		Env:       req.NewSpec.Env,
		Volumes:   req.NewSpec.Volumes,
		Ports:     req.NewSpec.Expose,
		Security:  req.NewSpec.Security,
		Resources: req.NewSpec.Resources,
		Labels: map[string]string{
			dockerlabels.LabelProject:  req.Project,
			dockerlabels.LabelService:  req.Service,
			dockerlabels.LabelReplica:  fmt.Sprintf("%d", req.ReplicaIdx),
			dockerlabels.LabelSpecHash: specHash,
		},
	})
	if err != nil {
		return fmt.Errorf("start-first: create %s: %w", tempName, err)
	}
	if err := req.Runtime.StartContainer(ctx, newID); err != nil {
		_ = req.Runtime.RemoveContainer(ctx, newID, true)
		return fmt.Errorf("start-first: start %s: %w", newID, err)
	}

	if err := req.Probes.Track(newID, req.NewSpec); err != nil {
		// Best-effort cleanup; the next tick will reconcile.
		_ = req.Runtime.RemoveContainer(ctx, newID, true)
		return fmt.Errorf("start-first: track %s: %w", newID, err)
	}

	healthy := waitForFirstHealthy(ctx, req.Probes, newID, probeDeadlineFor(req.NewSpec.Health))
	if !healthy {
		log.Warn("start-first: new container never reported healthy — rolling back",
			"project", req.Project, "service", req.Service, "replica", req.ReplicaIdx, "newID", newID)
		req.Probes.Untrack(newID)
		if err := req.Runtime.RemoveContainer(ctx, newID, true); err != nil {
			log.Error("start-first: rollback remove failed", "newID", newID, "err", err)
		}
		return ErrRolledBack
	}

	// Promote: remove old, rename new into the canonical name.
	if req.OldID != "" {
		req.Probes.Untrack(req.OldID)
		if err := req.Runtime.RemoveContainer(ctx, req.OldID, true); err != nil {
			log.Error("start-first: remove old failed (new is healthy, leaving in place)",
				"oldID", req.OldID, "err", err)
			return fmt.Errorf("start-first: remove old %s: %w", req.OldID, err)
		}
	}
	if err := req.Runtime.RenameContainer(ctx, newID, canonicalName); err != nil {
		// New is healthy but stuck under -new name; reconciler tick will not
		// be confused (label-based filter), so log and proceed.
		log.Warn("start-first: rename new to canonical failed", "newID", newID, "err", err)
	}
	log.Info("start-first: replacement complete", "project", req.Project,
		"service", req.Service, "replica", req.ReplicaIdx, "newID", newID)
	return nil
}

// StopFirst implements the data-safe rollover for stateful services.
// Old container is stopped + removed BEFORE the new is created — no
// two-writers window. Probe failure on the new container does NOT roll
// back (the old data may already be in flight); the next reconciler
// tick decides.
type StopFirst struct{}

// Name returns "stop-first".
func (*StopFirst) Name() string { return string(types.StrategyStopFirst) }

// Apply executes the stop-first contract.
func (*StopFirst) Apply(ctx context.Context, req Request) error {
	canonicalName := dockerlabels.ContainerNameFor(req.Project, req.Service, req.ReplicaIdx)
	log := req.Logger
	if log == nil {
		log = slog.Default()
	}
	const gracePeriod = 30 * time.Second

	if req.OldID != "" {
		if err := req.Runtime.StopContainer(ctx, req.OldID, gracePeriod); err != nil {
			return fmt.Errorf("stop-first: stop old %s: %w", req.OldID, err)
		}
		if err := waitForExit(ctx, req.Runtime, req.OldID, gracePeriod+5*time.Second); err != nil {
			log.Warn("stop-first: old never reported exited; force-removing", "oldID", req.OldID, "err", err)
		}
		req.Probes.Untrack(req.OldID)
		if err := req.Runtime.RemoveContainer(ctx, req.OldID, true); err != nil {
			return fmt.Errorf("stop-first: remove old %s: %w", req.OldID, err)
		}
	}

	if err := req.Runtime.PullImage(ctx, req.NewSpec.Image); err != nil {
		return fmt.Errorf("stop-first: pull %s: %w", req.NewSpec.Image, err)
	}
	specHash := hash.Hash(req.NewSpec)
	newID, err := req.Runtime.CreateContainer(ctx, rt.ContainerSpec{
		Name:      canonicalName,
		Image:     req.NewSpec.Image,
		Env:       req.NewSpec.Env,
		Volumes:   req.NewSpec.Volumes,
		Ports:     req.NewSpec.Expose,
		Security:  req.NewSpec.Security,
		Resources: req.NewSpec.Resources,
		Labels: map[string]string{
			dockerlabels.LabelProject:  req.Project,
			dockerlabels.LabelService:  req.Service,
			dockerlabels.LabelReplica:  fmt.Sprintf("%d", req.ReplicaIdx),
			dockerlabels.LabelSpecHash: specHash,
		},
	})
	if err != nil {
		return fmt.Errorf("stop-first: create %s: %w", canonicalName, err)
	}
	if err := req.Runtime.StartContainer(ctx, newID); err != nil {
		return fmt.Errorf("stop-first: start %s: %w", newID, err)
	}
	if err := req.Probes.Track(newID, req.NewSpec); err != nil {
		log.Warn("stop-first: track failed", "newID", newID, "err", err)
	}

	if healthy := waitForFirstHealthy(ctx, req.Probes, newID, probeDeadlineFor(req.NewSpec.Health)); !healthy {
		log.Warn("stop-first: new container never reported healthy — next tick will retry",
			"project", req.Project, "service", req.Service, "replica", req.ReplicaIdx, "newID", newID)
	}
	return nil
}

// waitForExit polls Runtime.InspectContainer until the container's
// State == "exited" or the timeout elapses.
func waitForExit(ctx context.Context, r rt.Runtime, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := r.InspectContainer(ctx, id)
		if err == nil && info != nil && info.State == "exited" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("container %s did not exit within %s", id, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// waitForFirstHealthy blocks until probe.Snapshot(id).HealthOK becomes
// true, or the deadline elapses, or ctx cancels.
func waitForFirstHealthy(ctx context.Context, probes *probe.Manager, id string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if snap, ok := probes.Snapshot(id); ok && snap.HealthOK && !snap.LastProbeAt.IsZero() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

// probeDeadlineFor computes the start-first rollover deadline as
// max(interval × (retries+1), 10s) — long enough for the new container
// to actually serve traffic, short enough that operators don't wait forever.
func probeDeadlineFor(h types.HealthCheck) time.Duration {
	interval := h.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	retries := h.Retries
	if retries <= 0 {
		retries = 3
	}
	d := interval * time.Duration(retries+1)
	if d < 10*time.Second {
		d = 10 * time.Second
	}
	return d
}
