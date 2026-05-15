package probe

import "sync"

// historyCap is the max entries the per-replica ring buffer holds.
// Bigger gives the dashboard more debugging context; smaller saves
// memory at 100-container scale. 32 is a comfortable middle.
const historyCap = 32

// History is a per-replica capped ring buffer of recent Results.
// Concurrency-safe; reads return copies.
type History struct {
	mu      sync.Mutex
	entries []Result
}

// Append records a new Result. If the buffer is full, the oldest
// entry is dropped (ring semantics).
func (h *History) Append(r Result) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, r)
	if len(h.entries) > historyCap {
		h.entries = h.entries[len(h.entries)-historyCap:]
	}
}

// Latest returns the most recent Result and true, or the zero Result
// and false when no probes have run yet.
func (h *History) Latest() (Result, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 {
		return Result{}, false
	}
	return h.entries[len(h.entries)-1], true
}

// Streak returns the count of consecutive failures at the tail of the
// buffer. Zero when the latest result is Healthy=true OR the buffer
// is empty.
func (h *History) Streak() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for i := len(h.entries) - 1; i >= 0; i-- {
		if h.entries[i].Healthy {
			break
		}
		n++
	}
	return n
}

// LastErr returns a string describing the latest failure, or empty
// when the latest result was healthy / there are no results.
func (h *History) LastErr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 {
		return ""
	}
	last := h.entries[len(h.entries)-1]
	if last.Healthy || last.Err == nil {
		return ""
	}
	return last.Err.Error()
}
