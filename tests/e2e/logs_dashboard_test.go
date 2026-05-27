//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_007_LogsDashboardSSE covers SC-007: the dashboard log viewer
// page loads + the SSE endpoint returns text/event-stream + data: lines
// flow within 2 seconds of a request that produces a new log line.
func TestSC_007_LogsDashboardSSE(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-007")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	hostPort, _ := harness.PickTwoFreeTCPPorts(t)
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
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-logdash-0").Run()
	})
	harness.WaitForCount(t, "logdash", 1, 30*time.Second)

	token := harness.ReadToken(t, dir)

	// --- Part 1: /ui/logs page loads with the right markup ---
	uiBody := harness.GetViaSocket(t, dir, token, "/ui/logs/default/logdash")
	for _, want := range []string{
		"📜",
		"logdash",
		"<select x-model.number=\"replica\"",
		"logsController",
		"new EventSource",
	} {
		if !strings.Contains(uiBody, want) {
			t.Errorf("/ui/logs page missing %q\n--- snippet ---\n%s", want, harness.Snippet(uiBody, 800))
		}
	}

	// --- Part 2: SSE endpoint returns text/event-stream + data: lines ---
	resp, body := harness.SSERequest(t, dir, token, "/api/v1/projects/default/services/logdash/logs?follow=true")
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
