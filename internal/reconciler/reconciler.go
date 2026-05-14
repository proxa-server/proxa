package reconciler

import (
	"context"
	"log/slog"
	"time"

	rt "github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// Reconciler is the long-running ticker loop that drives actual
// container state toward desired state. Stateless — restarts cheaply.
type Reconciler struct {
	store    store.StateStore
	runtime  rt.Runtime
	interval time.Duration
	poke     chan struct{}
	logger   *slog.Logger
}

// Options configure a Reconciler at construction time.
type Options struct {
	TickInterval time.Duration // default 5s
	Logger       *slog.Logger  // default slog.Default()
}

// New returns a Reconciler ready for Run.
func New(s store.StateStore, runtime rt.Runtime, opts Options) *Reconciler {
	if opts.TickInterval <= 0 {
		opts.TickInterval = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Reconciler{
		store:    s,
		runtime:  runtime,
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
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("reconciler started", "tickInterval", r.interval)
	defer r.logger.Info("reconciler stopped")

	tick := time.NewTicker(r.interval)
	defer tick.Stop()

	for {
		r.reconcileOnce(ctx)
		select {
		case <-ctx.Done():
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

	actions := Compute(desired, actual)
	if len(actions) == 0 {
		return
	}
	r.logger.Info("reconciling", "project", project, "actions", len(actions))

	for _, a := range actions {
		if err := Apply(ctx, r.runtime, a); err != nil {
			r.logger.Error("action failed", "type", a.Type, "project", a.Project,
				"service", a.Service, "replica", a.Replica, "reason", a.Reason, "err", err)
			continue
		}
		r.logger.Info("action applied", "type", a.Type, "project", a.Project,
			"service", a.Service, "replica", a.Replica, "reason", a.Reason)
	}
}

// _ keeps types reachable for future test helpers.
var _ types.ServiceStatus = ""
