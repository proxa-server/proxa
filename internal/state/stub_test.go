package state_test

import (
	"context"
	"errors"
	"testing"

	"github.com/proxa-server/proxa/internal/state"
)

func TestStub_AllMethodsReturnSentinel(t *testing.T) {
	s := state.NotImplementedSnapshot{}
	ctx := context.Background()

	if _, err := s.Take(ctx, "default"); !errors.Is(err, state.ErrSnapshotNotImplemented) {
		t.Errorf("Take err = %v, want ErrSnapshotNotImplemented", err)
	}
	if _, err := s.Load(ctx, "id"); !errors.Is(err, state.ErrSnapshotNotImplemented) {
		t.Errorf("Load err = %v, want ErrSnapshotNotImplemented", err)
	}
	if _, err := s.Diff("a", "b"); !errors.Is(err, state.ErrSnapshotNotImplemented) {
		t.Errorf("Diff err = %v, want ErrSnapshotNotImplemented", err)
	}
	if err := s.Restore(ctx, "id"); !errors.Is(err, state.ErrSnapshotNotImplemented) {
		t.Errorf("Restore err = %v, want ErrSnapshotNotImplemented", err)
	}
}
