package sqlite

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/proxa-server/proxa/internal/store"
)

// watchPollInterval is how often WatchServices re-queries SQLite for
// changes. Trades freshness for SQLite write contention. v1.0's etcd
// implementation will use a native watch instead.
const watchPollInterval = 2 * time.Second

// WatchServices emits events whenever a service in the given project
// is created, updated, or deleted. Detection is poll-based: every
// watchPollInterval the impl queries for services updated since the
// last poll and synthesizes events.
//
// The returned channel closes when ctx cancels. Consumers MUST reconcile
// against ListServices after any reconnect — events may be coalesced
// or dropped under load.
func (s *Store) WatchServices(ctx context.Context, project string) (<-chan store.ServiceEvent, error) {
	if project == "" {
		return nil, fmt.Errorf("store/sqlite: WatchServices requires non-empty project")
	}

	out := make(chan store.ServiceEvent, 16)
	go s.runWatch(ctx, project, out)
	return out, nil
}

func (s *Store) runWatch(ctx context.Context, project string, out chan<- store.ServiceEvent) {
	defer close(out)

	lastSeen := time.Time{}      // emit everything on the first tick
	known := map[string]string{} // service name -> last updated_at

	tick := time.NewTicker(watchPollInterval)
	defer tick.Stop()

	for {
		s.scanWatch(ctx, project, &lastSeen, known, out)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Store) scanWatch(ctx context.Context, project string, lastSeen *time.Time, known map[string]string, out chan<- store.ServiceEvent) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, updated_at FROM services WHERE project = ?`, project)
	if err != nil {
		return // transient error, retry next tick
	}
	current := map[string]string{}
	for rows.Next() {
		var name, updatedAt string
		if err := rows.Scan(&name, &updatedAt); err != nil {
			continue
		}
		current[name] = updatedAt
	}
	_ = rows.Close()

	// Detect created or updated.
	for name, updatedAt := range current {
		prev, existed := known[name]
		if !existed {
			svc, err := s.GetService(ctx, project, name)
			if err == nil {
				select {
				case out <- store.ServiceEvent{Type: "created", Project: project, Name: name, Service: svc}:
				case <-ctx.Done():
					return
				}
			}
		} else if prev != updatedAt {
			svc, err := s.GetService(ctx, project, name)
			if err == nil {
				select {
				case out <- store.ServiceEvent{Type: "updated", Project: project, Name: name, Service: svc}:
				case <-ctx.Done():
					return
				}
			}
		}
	}

	// Detect deleted.
	for name := range known {
		if _, stillThere := current[name]; !stillThere {
			select {
			case out <- store.ServiceEvent{Type: "deleted", Project: project, Name: name, Service: nil}:
			case <-ctx.Done():
				return
			}
		}
	}

	// Swap the known set.
	for k := range known {
		delete(known, k)
	}
	maps.Copy(known, current)
	*lastSeen = time.Now()
}
