package reconciler

import (
	"github.com/proxa-server/proxa/internal/probe"
	"github.com/proxa-server/proxa/pkg/types"
)

// Aggregate folds a service's desired replica count + per-replica probe
// snapshots into a single ServiceStatus. Pure function — no I/O, no
// locks. See specs/002-health-checks/data-model.md for the truth table.
//
//   - desired = 0 AND no replicas → stopped
//   - all desired replicas present AND every snapshot HealthOK         → healthy
//   - no healthy replicas                                              → failed
//   - some healthy < desired OR healthy < len(snapshots)                → degraded
//   - otherwise (transient — e.g., scale-up not yet converged)          → reconciling
func Aggregate(desired int, snapshots []probe.Snapshot) types.ServiceStatus {
	if desired == 0 && len(snapshots) == 0 {
		return types.ServiceStatusStopped
	}

	healthy := 0
	for _, s := range snapshots {
		if s.HealthOK {
			healthy++
		}
	}

	switch {
	case healthy == desired && healthy == len(snapshots):
		return types.ServiceStatusHealthy
	case healthy == 0:
		return types.ServiceStatusFailed
	case healthy < desired || healthy < len(snapshots):
		return types.ServiceStatusDegraded
	default:
		return types.ServiceStatusReconciling
	}
}
