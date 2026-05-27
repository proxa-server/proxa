package server

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	rt "github.com/proxa-server/proxa/internal/runtime"
)

// handleHostContainerLogs serves GET /api/v1/host-containers/{id}/logs.
// Plain-text by default, SSE when Accept: text/event-stream.
//
// Unlike the per-service log handler, there's no project/replica
// resolution — the {id} is the container id directly. Works for
// managed AND host containers (managed callers can still use the
// per-service endpoint if they prefer the meta envelope).
//
// Query params: tail (int, default 100), follow (bool, default false),
// since (RFC 3339, optional).
func (s *Server) handleHostContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "missing container id", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	tail, err := parseTail(q.Get("tail"))
	if err != nil {
		http.Error(w, "tail: "+err.Error(), http.StatusBadRequest)
		return
	}
	follow := q.Get("follow") == "true"
	var since time.Time
	if raw := q.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "since: must be RFC3339", http.StatusBadRequest)
			return
		}
		since = t
	}

	// Confirm the container exists so we return 404 (not 200-then-error)
	// for typo'd ids.
	info, err := s.runtime.InspectContainer(r.Context(), id)
	if err != nil || info == nil {
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}

	wantSSE := r.Header.Get("Accept") == "text/event-stream"
	if wantSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("X-Proxa-Container", info.Name)
	w.Header().Set("X-Proxa-Container-Id", shortID(id))
	w.WriteHeader(http.StatusOK)
	flush(w)
	if wantSSE {
		writeSSEEvent(w, "meta", fmt.Sprintf(`{"container":%q,"id":%q}`, info.Name, shortID(id)))
	}

	rc, err := s.runtime.StreamLogs(r.Context(), id, rt.LogOpts{
		Follow: follow,
		Tail:   tail,
		Since:  since,
	})
	if err != nil {
		if wantSSE {
			writeSSEEvent(w, "error", fmt.Sprintf(`{"code":"runtime-error","error":%q}`, err.Error()))
		} else {
			writePlainLine(w, "proxa: stream error: "+err.Error())
		}
		return
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if wantSSE {
			writeSSEData(w, line)
		} else {
			writePlainLine(w, line)
		}
	}
	if wantSSE {
		writeSSEEvent(w, "end", `{"reason":"stream closed"}`)
	}
}

// _ keeps strconv referenced even if signature changes (defensive — we
// don't currently use it, but parseTail's underlying signature might).
var _ = strconv.Atoi
