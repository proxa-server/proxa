package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/proxa-server/proxa/internal/events"
)

// handleListEvents responds to GET /api/v1/events.
//
// Query parameters:
//   - target  exact match (e.g., "service:default/api"); empty = any
//   - since   RFC 3339 lower bound on event time; empty = no lower bound
//   - limit   1..1000 (default 50)
//
// Returns 503 if the server has no events.Store wired (e.g., an older
// boot path or a unit test that omitted WithEvents).
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		http.Error(w, "events store not configured", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	filter := events.TailFilter{
		Target: q.Get("target"),
	}
	if raw := q.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "since: must be RFC3339", http.StatusBadRequest)
			return
		}
		filter.Since = t
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			http.Error(w, "limit: must be 1..1000", http.StatusBadRequest)
			return
		}
		filter.Limit = n
	}

	rows, err := s.events.Tail(r.Context(), filter)
	if err != nil {
		http.Error(w, "events: tail failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"events": rows,
	})
}
