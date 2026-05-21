package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SSE / chunked wire helpers used by handleStreamServiceLogs and the
// dashboard log viewer endpoint. Tiny — no framework needed.

// writeSSEEvent writes one "event: <e>\ndata: <d>\n\n" frame and
// flushes if w implements http.Flusher. Multi-line data is split into
// one "data: " field per line per the SSE spec.
func writeSSEEvent(w io.Writer, event, data string) {
	if event != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
	}
	for line := range strings.SplitSeq(data, "\n") {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
	_, _ = fmt.Fprint(w, "\n")
	flush(w)
}

// writeSSEData writes "data: <line>\n\n" + flush. Used for log lines
// where each input line is one event.
func writeSSEData(w io.Writer, line string) {
	_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
	flush(w)
}

// writePlainLine writes "<line>\n" + flush. Used for the chunked
// transfer path (CLI consumers).
func writePlainLine(w io.Writer, line string) {
	_, _ = fmt.Fprintf(w, "%s\n", line)
	flush(w)
}

func flush(w io.Writer) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
