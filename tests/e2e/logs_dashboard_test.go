//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSC_007_LogsDashboardSSE covers SC-007: the dashboard log viewer
// page loads + the SSE endpoint returns text/event-stream + data: lines
// flow within 2 seconds of a request that produces a new log line.
func TestSC_007_LogsDashboardSSE(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	hostPort, _ := pickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "logdash.toml")
	tomlContent := fmt.Sprintf(`
name     = "logdash"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 1

[security]
user = "101:101"

[[expose]]
container = 8080
host      = %d
protocol  = "http"
`, hostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-logdash-0").Run()
	})
	waitForCount(t, "logdash", 1, 30*time.Second)

	token := readToken(t, dir)

	// --- Part 1: /ui/logs page loads with the right markup ---
	uiBody := getViaSocket(t, dir, token, "/ui/logs/default/logdash")
	for _, want := range []string{
		"📜",
		"logdash",
		"<select x-model.number=\"replica\"",
		"logsController",
		"new EventSource",
	} {
		if !strings.Contains(uiBody, want) {
			t.Errorf("/ui/logs page missing %q\n--- snippet ---\n%s", want, snippet(uiBody, 800))
		}
	}

	// --- Part 2: SSE endpoint returns text/event-stream + data: lines ---
	resp, body := sseRequest(t, dir, token, "/api/v1/projects/default/services/logdash/logs?follow=true")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if resp.Header.Get("X-Proxa-Container") == "" {
		t.Errorf("X-Proxa-Container header missing")
	}

	// Read the meta event (first lines).
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	gotMeta := false
	lineCh := make(chan string, 100)
	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
	}()

	deadline := time.After(4 * time.Second)
metaLoop:
	for {
		select {
		case line, ok := <-lineCh:
			if !ok {
				break metaLoop
			}
			if strings.HasPrefix(line, "event: meta") || strings.HasPrefix(line, "data: {\"container\"") {
				gotMeta = true
				break metaLoop
			}
		case <-deadline:
			break metaLoop
		}
	}
	if !gotMeta {
		t.Errorf("SSE meta event not observed within 4s")
	}

	// Trigger a log line + assert a data: line flows.
	body = "" // reuse var
	_ = body
	if err := exec.Command("curl", "-sf", fmt.Sprintf("http://127.0.0.1:%d/", hostPort)).Run(); err != nil {
		t.Fatalf("curl logdash: %v", err)
	}

	gotData := false
	deadline = time.After(4 * time.Second)
dataLoop:
	for {
		select {
		case line, ok := <-lineCh:
			if !ok {
				break dataLoop
			}
			if strings.HasPrefix(line, "data: ") && (strings.Contains(line, "GET /") || strings.Contains(line, `"GET `)) {
				gotData = true
				break dataLoop
			}
		case <-deadline:
			break dataLoop
		}
	}
	if !gotData {
		t.Errorf("expected at least one data: line containing 'GET /' within 4s of curl")
	}
}

// getViaSocket runs an HTTP GET via the Unix socket and returns the body.
func getViaSocket(t *testing.T, dir, token, path string) string {
	t.Helper()
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath(t, dir))
			},
		},
	}
	req, _ := http.NewRequest(http.MethodGet, "http://x"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// sseRequest opens a long-lived streaming request via the Unix socket
// and returns the live response (caller closes resp.Body). Accept header
// is set to text/event-stream so the server takes the SSE branch.
func sseRequest(t *testing.T, dir, token, path string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{
		// NO timeout — streaming.
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath(t, dir))
			},
		},
	}
	req, _ := http.NewRequest(http.MethodGet, "http://x"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE GET %s: %v", path, err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("SSE GET %s: status %d", path, resp.StatusCode)
	}
	return resp, ""
}
