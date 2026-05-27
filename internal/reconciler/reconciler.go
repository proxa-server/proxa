package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/proxa-server/proxa/internal/events"
	"github.com/proxa-server/proxa/internal/ingress"
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
	ingress  ingress.IngressController
	events   *events.Store // optional; nil => no event audit
	interval time.Duration
	poke     chan struct{}
	logger   *slog.Logger
}

// Options configure a Reconciler at construction time.
type Options struct {
	TickInterval time.Duration             // default 5s
	Logger       *slog.Logger              // default slog.Default()
	Probes       *probe.Manager            // required; pass probe.New(runtime, logger)
	Ingress      ingress.IngressController // optional; reconciler skips ingress push when nil
	Events       *events.Store             // optional; v0.4.3+ audit log; nil = silent
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
		ingress:  opts.Ingress,
		events:   opts.Events,
		interval: opts.TickInterval,
		poke:     make(chan struct{}, 1),
		logger:   opts.Logger,
	}
}

// emitEvent writes one event row. Best-effort: failures are logged but
// never propagated — an event store outage must not block the
// reconciler tick. No-op when r.events is nil (production server wires
// one in, but unit tests don't always).
func (r *Reconciler) emitEvent(ctx context.Context, typ, target, payload string) {
	if r.events == nil {
		return
	}
	if _, err := r.events.Append(ctx, events.Event{
		Type:    typ,
		Actor:   events.ActorReconciler,
		Target:  target,
		Payload: payload,
	}); err != nil {
		r.logger.Warn("events.Append failed", "type", typ, "target", target, "err", err)
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
// project. Routes are pushed BEFORE per-project work so any probe that
// opts into via=ingress has the route table available on its first
// attempt (FR-010 + SC-007). Backends are pushed inside reconcileProject
// before probe Track, same reason.
func (r *Reconciler) reconcileOnce(ctx context.Context) {
	projects, err := r.store.ListProjects(ctx)
	if err != nil {
		r.logger.Error("list projects", "err", err)
		return
	}

	if r.ingress != nil {
		r.pushRoutes(ctx, projects)
	}

	for _, p := range projects {
		r.reconcileProject(ctx, p.Name)
	}
}

// pushRoutes collects every service's [[route]] declarations across
// every project and publishes them as a single atomic snapshot to the
// ingress. Empty maps are valid (means "no routes" → ingress returns
// 404 for every request).
func (r *Reconciler) pushRoutes(ctx context.Context, projects []types.Project) {
	routes := make(map[ingress.ServiceID][]types.Route)
	for _, p := range projects {
		services, err := r.store.ListServices(ctx, p.Name)
		if err != nil {
			r.logger.Error("list services for route push", "project", p.Name, "err", err)
			continue
		}
		for _, svc := range services {
			if len(svc.Spec.Routes) == 0 {
				continue
			}
			routes[ingress.ServiceID{Project: p.Name, Service: svc.Name}] = svc.Spec.Routes
		}
	}
	if err := r.ingress.UpdateRoutes(ctx, routes); err != nil {
		r.logger.Error("ingress: UpdateRoutes rejected", "err", err)
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
			r.emitActionEvent(ctx, a)
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

	// Pass 1: collect IDs per service. NO Track yet — we want backends
	// pushed to ingress first so probe-via-ingress works on first hit.
	for _, c := range actual {
		if !isActive(c.State) {
			continue
		}
		p := c.Labels[dockerlabels.LabelProject]
		s := c.Labels[dockerlabels.LabelService]
		if p != project {
			continue
		}
		if _, ok := specByService[s]; !ok {
			continue // container belongs to a deleted service; let the next tick remove it
		}
		idsByService[s] = append(idsByService[s], c.ID)
		activeIDs[c.ID] = struct{}{}
	}

	// Pass 2: push backends to ingress for every service that has
	// routes (must happen BEFORE probe Track for FR-010 / SC-007).
	if r.ingress != nil {
		for _, svc := range desired {
			if len(svc.Spec.Routes) == 0 {
				continue
			}
			r.pushBackends(ctx, project, svc, idsByService[svc.Name])
		}
	}

	// Pass 3: Track probes (now safe — ingress has routes + backends).
	for _, c := range actual {
		if !isActive(c.State) {
			continue
		}
		s := c.Labels[dockerlabels.LabelService]
		spec, ok := specByService[s]
		if !ok {
			continue
		}
		if err := r.probes.Track(c.ID, spec); err != nil {
			r.logger.Warn("probe track failed", "container", c.ID, "err", err)
		}
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

	// Pass 4: aggregate + persist Service.Status when it changed.
	for _, svc := range desired {
		ids := idsByService[svc.Name]
		snaps := make([]probe.Snapshot, 0, len(ids))
		for _, id := range ids {
			if s, ok := r.probes.Snapshot(id); ok {
				snaps = append(snaps, s)
			}
		}
		newStatus := Aggregate(svc.Spec.Replicas, snaps)
		if newStatus != svc.Status {
			prev := svc.Status
			svc.Status = newStatus
			if err := r.store.PutService(ctx, project, svc); err != nil {
				r.logger.Error("persist service status", "project", project, "service", svc.Name, "err", err)
			} else {
				r.logger.Info("service status changed", "project", project, "service", svc.Name, "status", newStatus)
				r.emitEvent(ctx, events.TypeServiceStatusChanged,
					events.TargetService(project, svc.Name),
					fmt.Sprintf(`{"from":%q,"to":%q}`, prev, newStatus))
			}
		}
	}
}

// pushBackends builds the Backend list for one service and publishes
// it to the ingress.
//
// Reachability resolution per replica:
//   - If the spec's first [[expose]] declares host > 0, the ingress
//     dials 127.0.0.1:<hostPort>. Works on Docker Desktop (macOS /
//     Windows) where the bridge subnet is unreachable from the host.
//   - Otherwise the ingress dials the container's bridge IP. Required
//     for multi-replica services on Linux (only one container can bind
//     a host port at a time).
func (r *Reconciler) pushBackends(ctx context.Context, project string, svc types.Service, ids []string) {
	ip, port := backendDial(svc.Spec)
	backends := make([]ingress.Backend, 0, len(ids))
	for _, id := range ids {
		snap, tracked := r.probes.Snapshot(id)
		// Optimistic healthy on first sight: a brand-new container has no
		// snapshot yet (Track hasn't fired its first probe), so default to
		// healthy. Otherwise probe-via-ingress can never bootstrap — the
		// probe gets 503 (no healthy backend) → never succeeds → backend
		// stays unhealthy forever (SC-007 deadlock without this).
		healthy := !tracked || snap.HealthOK
		b := ingress.Backend{
			ContainerID: id,
			Port:        port,
			Healthy:     healthy,
		}
		if ip != "" {
			// Shortcut: host-port dial.
			b.IPAddress = ip
		} else {
			info, err := r.runtime.InspectContainer(ctx, id)
			if err != nil || info == nil || info.IPAddress == "" {
				continue // unreachable replica; skip
			}
			b.IPAddress = info.IPAddress
		}
		backends = append(backends, b)
	}
	r.ingress.UpdateBackends(ctx, ingress.ServiceID{Project: project, Service: svc.Name}, backends)
}

// backendDial returns (ip, port) for the ingress to dial when routing
// to a replica of spec. Empty ip means "use the container's bridge IP".
//
//	(ip, port) = ("127.0.0.1", expose[0].Host)  when first expose has host > 0
//	(ip, port) = ("", containerPort)            otherwise (bridge IP at dial time)
//
// containerPort is, in order: spec.Health.Port → spec.Expose[0].Container → 80.
func backendDial(spec types.TaskDef) (string, int) {
	containerPort := 80
	switch {
	case spec.Health.Port > 0:
		containerPort = spec.Health.Port
	case len(spec.Expose) > 0 && spec.Expose[0].Container > 0:
		containerPort = spec.Expose[0].Container
	}
	if len(spec.Expose) > 0 && spec.Expose[0].Host > 0 {
		return "127.0.0.1", spec.Expose[0].Host
	}
	return "", containerPort
}

// emitActionEvent maps a reconciler Action onto an audit event. Called
// only after Apply returns nil — failed actions are surfaced via logs
// (audit-trail of attempted-but-failed work is intentionally out of
// scope for v0.4.3; revisit when scheduler dead-letter lands in v0.5).
func (r *Reconciler) emitActionEvent(ctx context.Context, a Action) {
	target := events.TargetService(a.Project, a.Service)
	payload := fmt.Sprintf(`{"replica":%d,"reason":%q}`, a.Replica, a.Reason)
	switch a.Type {
	case ActionCreate:
		r.emitEvent(ctx, events.TypeReconcilerCreate, target, payload)
	case ActionRemove:
		r.emitEvent(ctx, events.TypeReconcilerRemove, target, payload)
	case ActionReplace:
		// Replace is implemented as a stop-old + start-new pair; the
		// event semantics that operators want to see are "we restarted
		// replica N because <reason>" — model it as a scale event with
		// payload distinguishing the cause.
		r.emitEvent(ctx, events.TypeReconcilerScale, target, payload)
	}
}

// _ keeps types reachable for future test helpers.
var _ types.ServiceStatus = ""
