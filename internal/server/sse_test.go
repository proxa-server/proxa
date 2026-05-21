package server

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
)

// flushCounter wraps a bytes.Buffer with a flush count for assertions.
type flushCounter struct {
	bytes.Buffer
	flushes atomic.Int64
}

func (f *flushCounter) Flush() { f.flushes.Add(1) }

func TestWriteSSEData(t *testing.T) {
	var f flushCounter
	writeSSEData(&f, "hello world")
	got := f.String()
	want := "data: hello world\n\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if f.flushes.Load() != 1 {
		t.Errorf("flushes = %d, want 1", f.flushes.Load())
	}
}

func TestWriteSSEEventSplitsMultilineData(t *testing.T) {
	var f flushCounter
	writeSSEEvent(&f, "meta", "first\nsecond\nthird")
	got := f.String()
	want := "event: meta\ndata: first\ndata: second\ndata: third\n\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if f.flushes.Load() != 1 {
		t.Errorf("flushes = %d, want 1", f.flushes.Load())
	}
}

func TestWriteSSEEventEmptyEventName(t *testing.T) {
	var f flushCounter
	writeSSEEvent(&f, "", "raw")
	got := f.String()
	// No event: header when event="".
	want := "data: raw\n\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWritePlainLine(t *testing.T) {
	var f flushCounter
	writePlainLine(&f, "one line")
	if f.String() != "one line\n" {
		t.Errorf("got %q, want \"one line\\n\"", f.String())
	}
	if f.flushes.Load() != 1 {
		t.Errorf("flushes = %d, want 1", f.flushes.Load())
	}
}

func TestFlushNoOpOnNonFlusher(t *testing.T) {
	var b bytes.Buffer     // doesn't implement Flusher
	writeSSEData(&b, "ok") // must not panic
	if !strings.Contains(b.String(), "ok") {
		t.Errorf("data not written: %q", b.String())
	}
}

func TestFlushPerWrite(t *testing.T) {
	var f flushCounter
	for range 5 {
		writeSSEData(&f, "line")
	}
	if f.flushes.Load() != 5 {
		t.Errorf("expected 5 flushes (one per line), got %d", f.flushes.Load())
	}
}
