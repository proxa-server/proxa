package reconciler

import (
	"context"
	"log/slog"
	"time"

	"github.com/proxa-server/proxa/internal/probe"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// Reconciler is the long-running ticker loop that drives actual
// container state toward desired state. Stateless — restarts cheaply.
type Reconciler struct {
	store    store.StateStore
	runtime  rt.Runtime
	probes   *probe.Manager
	interval time.Duration
	poke     chan struct{}
	logger   *slog.Logger
}

// Options configure a Reconciler at construction time.
type Options struct {
	TickInterval time.Duration  // default 5s
	Logger       *slog.Logger   // default slog.Default()
	Probes       *probe.Manager // required; pass probe.New(runtime, logger)
}

// New returns a Reconciler ready for Run.
func New(s store.StateStore, runtime rt.Runtime, opts Options) *Reconciler {
	if opts.TickInterval <= 0 {
		opts.TickInterval = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	probes := opts.Probes
	if probes == nil {
		probes = probe.New(runtime, opts.Logger)
	}
	return &Reconciler{
		store:    s,
		runtime:  runtime,
		probes:   probes,
		interval: opts.TickInterval,
		poke:     make(chan struct{}, 1),
		logger:   opts.Logger,
	}
}

// Poke triggers a reconciliation tick before the next scheduled one.
// Non-blocking — drops if a poke is already pending.
func (r *Reconciler) Poke() {
	select {
	case r.poke <- struct{}{}:
	default:
	}
}

// Run blocks until ctx cancels, ticking the reconciler at TickInterval
// and on every Poke. Per-action errors are logged and the loop continues.
// The probe manager runs in a sibling goroutine and shuts down with ctx.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("reconciler started", "tickInterval", r.interval)
	defer r.logger.Info("reconciler stopped")

	probesDone := make(chan struct{})
	go func() {
		r.probes.Run(ctx)
		close(probesDone)
	}()

	tick := time.NewTicker(r.interval)
	defer tick.Stop()

	for {
		r.reconcileOnce(ctx)
		select {
		case <-ctx.Done():
			<-probesDone
			return
		case <-tick.C:
		case <-r.poke:
		}
	}
}

// reconcileOnce performs one full reconciliation pass across every
// project. Errors are logged per-project and per-action.
func (r *Reconciler) reconcileOnce(ctx context.Context) {
	projects, err := r.store.ListProjects(ctx)
	if err != nil {
		r.logger.Error("list projects", "err", err)
		return
	}

	for _, p := range projects {
		r.reconcileProject(ctx, p.Name)
	}
}

func (r *Reconciler) reconcileProject(ctx context.Context, project string) {
	desired, err := r.store.ListServices(ctx, project)
	if err != nil {
		r.logger.Error("list services", "project", project, "err", err)
		return
	}

	actual, err := r.runtime.ListContainers(ctx, rt.ListFilter{Project: project})
	if err != nil {
		r.logger.Error("list containers", "project", project, "err", err)
		return
	}

	actions := Compute(desired, actual, r.probeUnhealthySet(desired, actual))
	if len(actions) > 0 {
		r.logger.Info("reconciling", "project", project, "actions", len(actions))
		for _, a := range actions {
			if err := Apply(ctx, r.runtime, r.probes, r.logger, a); err != nil {
				r.logger.Error("action failed", "type", a.Type, "project", a.Project,
					"service", a.Service, "replica", a.Replica, "reason", a.Reason, "err", err)
				continue
			}
			r.logger.Info("action applied", "type", a.Type, "project", a.Project,
				"service", a.Service, "replica", a.Replica, "reason", a.Reason)
		}
		// Refresh actual state so probe wiring + status aggregation see
		// containers we just created.
		actual, err = r.runtime.ListContainers(ctx, rt.ListFilter{Project: project})
		if err != nil {
			r.logger.Error("list containers (post-actions)", "project", project, "err", err)
			return
		}
	}

	r.updateProbesAndStatus(ctx, project, desired, actual)
}

// probeUnhealthySet returns the set of containerIDs whose probe streak
// has hit-or-exceeded the service's configured retries. The reconciler
// passes this to Compute so unhealthy replicas get rotated like crashed
// ones (FR-006). Containers without a tracked probe (e.g., no [health]
// block) never appear in the result.
func (r *Reconciler) probeUnhealthySet(desired []types.Service, actual []rt.ContainerInfo) map[string]bool {
	retriesByService := make(map[string]int, len(desired))
	for _, svc := range desired {
		retries := svc.Spec.Health.Retries
		if retries <= 0 {
			retries = 3
		}
		retriesByService[svc.Name] = retries
	}
	out := make(map[string]bool)
	for _, c := range actual {
		svcName := c.Labels[dockerlabels.LabelService]
		retries, ok := retriesByService[svcName]
		if !ok {
			continue
		}
		snap, tracked := r.probes.Snapshot(c.ID)
		if !tracked || snap.HealthOK {
			continue
		}
		if snap.Streak >= retries {
			out[c.ID] = true
		}
	}
	return out
}

// updateProbesAndStatus tracks every active container, untracks any
// container that is no longer present in this project's actual state,
// and persists a fresh Service.Status when it has changed.
func (r *Reconciler) updateProbesAndStatus(ctx context.Context, project string, desired []types.Service, actual []rt.ContainerInfo) {
	specByService := make(map[string]types.TaskDef, len(desired))
	for _, svc := range desired {
		specByService[svc.Name] = svc.Spec
	}

	idsByService := make(map[string][]string, len(desired))
	activeIDs := make(map[string]struct{}, len(actual))

	for _, c := range actual {
		if !isActive(c.State) {
			continue
		}
		p := c.Labels[dockerlabels.LabelProject]
		s := c.Labels[dockerlabels.LabelService]
		if p != project {
			continue
		}
		spec, ok := specByService[s]
		if !ok {
			continue // container belongs to a deleted service; let the next tick remove it
		}
		if err := r.probes.Track(c.ID, spec); err != nil {
			r.logger.Warn("probe track failed", "container", c.ID, "err", err)
			continue
		}
		idsByService[s] = append(idsByService[s], c.ID)
		activeIDs[c.ID] = struct{}{}
	}

	// Untrack any container that has disappeared from THIS project.
	for _, id := range r.probes.TrackedIDs() {
		if _, ok := activeIDs[id]; ok {
			continue
		}
		// Only untrack if the container belonged to this project — we
		// don't want one project's tick to disturb another's tracking.
		// Cheapest check: scan the actual list again for matching ID.
		belongs := false
		for _, c := range actual {
			if c.ID == id {
				belongs = true
				break
			}
		}
		if belongs {
			r.probes.Untrack(id)
		}
	}

	// Aggregate + persist Service.Status when it changed.
	for _, svc := range desired {
		ids := idsByService[svc.Name]
		snaps := make([]probe.Snapshot, 0, len(ids))
		for _, id := range ids {
			if s, ok := r.probes.Snapshot(id); ok {
				snaps = append(snaps, s)
			}
		}
		newStatus := Aggregate(svc.Spec.Replicas, snaps)
		if newStatus == svc.Status {
			continue
		}
		svc.Status = newStatus
		if err := r.store.PutService(ctx, project, svc); err != nil {
			r.logger.Error("persist service status", "project", project, "service", svc.Name, "err", err)
			continue
		}
		r.logger.Info("service status changed", "project", project, "service", svc.Name, "status", newStatus)
	}
}

// _ keeps types reachable for future test helpers.
var _ types.ServiceStatus = ""
