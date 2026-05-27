package state

import "context"

// NotImplementedSnapshot is the v0.4.3 stub. Every method returns
// ErrSnapshotNotImplemented. Used by callers that need to compile
// against the Snapshot contract before v0.5 lands the real impl.
type NotImplementedSnapshot struct{}

// Compile-time assertion that the stub satisfies the interface.
var _ Snapshot = NotImplementedSnapshot{}

func (NotImplementedSnapshot) Take(context.Context, string) (SnapshotID, error) {
	return "", ErrSnapshotNotImplemented
}

func (NotImplementedSnapshot) Load(context.Context, SnapshotID) (*ProjectState, error) {
	return nil, ErrSnapshotNotImplemented
}

func (NotImplementedSnapshot) Diff(SnapshotID, SnapshotID) (Diff, error) {
	return Diff{}, ErrSnapshotNotImplemented
}

func (NotImplementedSnapshot) Restore(context.Context, SnapshotID) error {
	return ErrSnapshotNotImplemented
}
