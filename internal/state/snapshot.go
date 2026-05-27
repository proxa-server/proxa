// Package state holds the cross-version state-snapshot contract used
// by v0.5 Confidence Mode (rollback + diff) and v0.6 Post-Mortem Mode
// (time-travel). v0.4.3 ships the interface + a stub implementation
// returning ErrSnapshotNotImplemented; v0.5 lands the real impl.
//
// The contract is LOCKED in v0.4.3 so v0.5 (and any other consumer)
// can compile against it immediately — only the implementation binary
// changes when v0.5 lands.
package state

import (
	"context"
	"errors"
	"time"
)

// ErrSnapshotNotImplemented is returned by the v0.4.3 stub from every
// Snapshot method. Callers detect via errors.Is and surface a "this
// feature lands in v0.5" message instead of panicking.
var ErrSnapshotNotImplemented = errors.New("state: snapshot not implemented (v0.4.3 stub; real impl arrives v0.5)")

// SnapshotID identifies one captured project state. Format is opaque;
// v0.5 will use a timestamp + random suffix for uniqueness across
// concurrent captures.
type SnapshotID string

// ProjectState is the captured state of one project at a moment in
// time. Loose schema in v0.4.3 — v0.5 fleshes out the fields when the
// real Snapshot.Load lands.
type ProjectState struct {
	ID       SnapshotID `json:"id"`
	Project  string     `json:"project"`
	TakenAt  time.Time  `json:"taken_at"`
	Services []byte     `json:"services"` // JSON blob of the project's services at capture time
	Routes   []byte     `json:"routes"`   // JSON blob of the project's ingress routes
}

// Diff describes the delta between two ProjectStates. v0.5 expands the
// fields; v0.4.3 only locks the type name + signature.
type Diff struct {
	From SnapshotID `json:"from"`
	To   SnapshotID `json:"to"`
	// Lines is the human-readable unified-diff representation v0.5
	// will populate. Empty in the stub.
	Lines []string `json:"lines"`
}

// Snapshot is the v0.5 Confidence Mode + v0.6 Post-Mortem Mode
// contract. Every method's signature is committed; v0.4.3's stub
// returns ErrSnapshotNotImplemented uniformly.
type Snapshot interface {
	// Take captures the current state of the named project and returns
	// the resulting SnapshotID. Idempotent w.r.t. project + timestamp
	// in v0.5's impl (no duplicate captures within a 1s window).
	Take(ctx context.Context, project string) (SnapshotID, error)

	// Load returns the captured ProjectState for the given ID, or an
	// error if the snapshot does not exist.
	Load(ctx context.Context, id SnapshotID) (*ProjectState, error)

	// Diff computes the delta between two SnapshotIDs.
	Diff(a, b SnapshotID) (Diff, error)

	// Restore reverts the live project state to the captured snapshot.
	// THIS IS DESTRUCTIVE — the reconciler will reconcile the live
	// state to match the snapshot, removing services that weren't in
	// the snapshot and re-creating ones that were.
	Restore(ctx context.Context, id SnapshotID) error
}
