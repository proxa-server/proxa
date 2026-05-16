// Package probe runs per-replica health checks (HTTP, exec) for the
// reconciler. One goroutine per tracked container; results land in an
// in-memory snapshot the reconciler reads each tick.
//
// See specs/002-health-checks/contracts/probe.md for the full
// behavioral contract.
package probe

import (
	"context"
	"time"
)

// Probe is the contract every probe implementation satisfies.
// One Probe instance per replica per probe type (HTTP, exec).
type Probe interface {
	// Name identifies the probe type ("http", "exec").
	Name() string

	// Run executes one probe attempt. Returns Healthy=true on success;
	// Healthy=false with a non-nil Err on failure. The implementation
	// MUST honor ctx cancellation and the configured per-probe timeout.
	Run(ctx context.Context) Result
}

// Result is one probe outcome.
type Result struct {
	At      time.Time
	Healthy bool
	Latency time.Duration
	Err     error
}
