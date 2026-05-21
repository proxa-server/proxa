package docker

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	rt "github.com/proxa-server/proxa/internal/runtime"
)

// stdcopyFrame builds one stdcopy-multiplexed frame.
// stream=1 → stdout, 2 → stderr.
func stdcopyFrame(stream byte, payload string) []byte {
	hdr := [8]byte{stream, 0, 0, 0, 0, 0, 0, 0}
	n := len(payload)
	hdr[4] = byte(n >> 24)
	hdr[5] = byte(n >> 16)
	hdr[6] = byte(n >> 8)
	hdr[7] = byte(n)
	out := make([]byte, 0, 8+n)
	out = append(out, hdr[:]...)
	out = append(out, payload...)
	return out
}

func TestStreamLogsDemuxesSingleStdoutFrame(t *testing.T) {
	m := &mockDockerClient{
		logsBody: stdcopyFrame(1, "hello world\n"),
	}
	r := runtimeWithMock(m)

	rc, err := r.StreamLogs(context.Background(), "c1", rt.LogOpts{Tail: -1})
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	defer rc.Close()

	buf, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "hello world\n" {
		t.Errorf("demuxed = %q, want %q", buf, "hello world\n")
	}
}

func TestStreamLogsDemuxesInterleavedStdoutStderr(t *testing.T) {
	body := append(stdcopyFrame(1, "out1\n"), stdcopyFrame(2, "err1\n")...)
	body = append(body, stdcopyFrame(1, "out2\n")...)
	m := &mockDockerClient{logsBody: body}
	r := runtimeWithMock(m)

	rc, err := r.StreamLogs(context.Background(), "c1", rt.LogOpts{})
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	defer rc.Close()

	buf, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// stdcopy.StdCopy with the same writer for both streams produces
	// the bytes in input order.
	want := "out1\nerr1\nout2\n"
	if string(buf) != want {
		t.Errorf("demuxed = %q, want %q", buf, want)
	}
}

func TestStreamLogsCloseStopsGoroutine(t *testing.T) {
	// Body big enough that StdCopy is still working when Close fires.
	body := []byte{}
	for i := 0; i < 100; i++ {
		body = append(body, stdcopyFrame(1, "line\n")...)
	}
	m := &mockDockerClient{logsBody: body}
	r := runtimeWithMock(m)

	before := runtime.NumGoroutine()
	rc, err := r.StreamLogs(context.Background(), "c1", rt.LogOpts{})
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	// Read a little so the goroutine is mid-StdCopy, then close.
	tmp := make([]byte, 8)
	_, _ = rc.Read(tmp)
	rc.Close()

	// Give the goroutine a moment to exit via the broken-pipe path.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+1 { // +1 tolerance for test runner noise
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("demux goroutine leaked after Close: before=%d now=%d", before, runtime.NumGoroutine())
}

func TestStreamLogsEmptyContainerID(t *testing.T) {
	r := runtimeWithMock(&mockDockerClient{})
	_, err := r.StreamLogs(context.Background(), "", rt.LogOpts{})
	if err == nil || !strings.Contains(err.Error(), "container id required") {
		t.Errorf("expected container-id-required error, got %v", err)
	}
}

func TestStreamLogsDaemonError(t *testing.T) {
	m := &mockDockerClient{logsErr: errors.New("daemon down")}
	r := runtimeWithMock(m)
	_, err := r.StreamLogs(context.Background(), "c1", rt.LogOpts{})
	if err == nil || !strings.Contains(err.Error(), "daemon down") {
		t.Errorf("expected daemon error to propagate, got %v", err)
	}
}

func TestTailValue(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{-1, "all"},
		{-99, "all"},
		{0, "0"},
		{1, "1"},
		{100, "100"},
	}
	for _, c := range cases {
		got := tailValue(c.in)
		if got != c.want {
			t.Errorf("tailValue(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
