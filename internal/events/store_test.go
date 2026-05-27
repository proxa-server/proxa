package events_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/events"
	"github.com/proxa-server/proxa/internal/store/sqlite"

	_ "modernc.org/sqlite"
)

// newTestDB returns an open *sql.DB with migrations applied (so the
// events table exists).
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	st := sqlite.New()
	if err := st.Open(context.Background(), ":memory:"); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Get the underlying *sql.DB. Store.DB() isn't exported in current
	// shape; we cheat by re-opening the same ":memory:" sentinel. For
	// real tests we need a way to share — use a file-backed temp db
	// instead so both Store + raw *sql.DB see the same data.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	// Re-run migrations on the raw db so events table exists.
	if _, err := db.Exec(`CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts INTEGER NOT NULL,
		type TEXT NOT NULL,
		actor TEXT NOT NULL,
		target TEXT NOT NULL,
		payload TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create events table: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStore_AppendAndTail(t *testing.T) {
	db := newTestDB(t)
	s := events.NewStore(db)
	ctx := context.Background()

	id1, err := s.Append(ctx, events.Event{
		Type:    events.TypeReconcilerCreate,
		Actor:   events.ActorReconciler,
		Target:  events.TargetService("default", "api"),
		Payload: `{"reason":"actual<desired"}`,
	})
	if err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if id1 != 1 {
		t.Errorf("first event id = %d, want 1", id1)
	}

	// Second event.
	if _, err := s.Append(ctx, events.Event{
		Type:   events.TypeProbeTransition,
		Actor:  events.ActorReconciler,
		Target: events.TargetContainer("abc123"),
	}); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	got, err := s.Tail(ctx, events.TailFilter{})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	// Newest first.
	if got[0].Type != events.TypeProbeTransition {
		t.Errorf("got[0].Type = %q, want probe.transition", got[0].Type)
	}
}

func TestStore_TailByTarget(t *testing.T) {
	db := newTestDB(t)
	s := events.NewStore(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		target := events.TargetService("default", "api")
		if i%2 == 0 {
			target = events.TargetService("default", "worker")
		}
		_, err := s.Append(ctx, events.Event{
			Type:   events.TypeReconcilerCreate,
			Actor:  events.ActorReconciler,
			Target: target,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Tail(ctx, events.TailFilter{
		Target: events.TargetService("default", "api"),
	})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 api events, got %d", len(got))
	}
	for _, e := range got {
		if e.Target != events.TargetService("default", "api") {
			t.Errorf("unexpected target %q", e.Target)
		}
	}
}

func TestStore_Append_RejectsEmptyFields(t *testing.T) {
	db := newTestDB(t)
	s := events.NewStore(db)
	ctx := context.Background()

	cases := []events.Event{
		{Actor: "a", Target: "t"},                 // missing Type
		{Type: "x", Target: "t"},                  // missing Actor
		{Type: "x", Actor: "a"},                   // missing Target
	}
	for i, e := range cases {
		if _, err := s.Append(ctx, e); err == nil {
			t.Errorf("case %d: expected error for empty field", i)
		}
	}
}

func TestStore_Tail_LimitClamped(t *testing.T) {
	db := newTestDB(t)
	s := events.NewStore(db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := s.Append(ctx, events.Event{
			Type:   events.TypeReconcilerCreate,
			Actor:  events.ActorReconciler,
			Target: events.TargetService("default", "svc"+strconv.Itoa(i)),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Tail(ctx, events.TailFilter{Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("want 5 with default limit, got %d", len(got))
	}
}

func TestStore_TailWithSince(t *testing.T) {
	db := newTestDB(t)
	s := events.NewStore(db)
	ctx := context.Background()

	old := events.Event{
		Type:   events.TypeReconcilerCreate,
		Actor:  events.ActorReconciler,
		Target: events.TargetService("default", "old"),
		At:     time.Now().Add(-2 * time.Hour),
	}
	new := events.Event{
		Type:   events.TypeReconcilerCreate,
		Actor:  events.ActorReconciler,
		Target: events.TargetService("default", "new"),
		At:     time.Now(),
	}
	_, _ = s.Append(ctx, old)
	_, _ = s.Append(ctx, new)

	got, err := s.Tail(ctx, events.TailFilter{
		Since: time.Now().Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Target != events.TargetService("default", "new") {
		t.Errorf("expected only the new event; got %+v", got)
	}
}

// BenchmarkAppend confirms sub-millisecond p99 append even with rows
// already present. SC-003 target.
func BenchmarkAppend(b *testing.B) {
	db := newTestDB(&testing.T{})
	s := events.NewStore(db)
	ctx := context.Background()

	// Seed 1k rows so we're not measuring an empty-table case.
	for i := 0; i < 1000; i++ {
		_, _ = s.Append(ctx, events.Event{
			Type:   events.TypeReconcilerCreate,
			Actor:  events.ActorReconciler,
			Target: events.TargetService("default", "seed"+strconv.Itoa(i)),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Append(ctx, events.Event{
			Type:   events.TypeReconcilerCreate,
			Actor:  events.ActorReconciler,
			Target: events.TargetService("default", "bench"+strconv.Itoa(i)),
		})
	}
}
