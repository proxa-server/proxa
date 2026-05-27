//go:build e2e
// +build e2e

// Run with: make test-e2e
//
// Covers SC-001 / SC-011 of specs/007-architectural-foundations: every
// reconciler create action must land an event in the audit log,
// retrievable via /api/v1/events.

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

func TestSC_007_EventsEmittedOnReconcilerCreate(t *testing.T) {
	harness.SCAttrs(t, "007-architectural-foundations", "SC-001")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "evtsvc.toml")
	tomlContent := `
name     = "evtsvc"
image    = "nginx:alpine"
replicas = 1

[[expose]]
container = 80
host      = 0
protocol  = "http"
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-evtsvc-0").Run()
	})

	// Wait for reconciler to converge + emit events.
	time.Sleep(7 * time.Second)

	token := harness.ReadToken(t, dir)
	body := harness.GetViaSocket(t, dir, token, "/api/v1/events?limit=100")

	var payload struct {
		Events []struct {
			Type   string `json:"type"`
			Actor  string `json:"actor"`
			Target string `json:"target"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode events response: %v\n%s", err, body)
	}
	if len(payload.Events) == 0 {
		t.Fatalf("no events returned from /api/v1/events; body=%s", body)
	}

	wantTarget := "service:default/evtsvc"
	gotCreate := false
	for _, e := range payload.Events {
		if e.Type == "reconciler.create" && e.Target == wantTarget && e.Actor == "reconciler" {
			gotCreate = true
			break
		}
	}
	if !gotCreate {
		summary := make([]string, 0, len(payload.Events))
		for _, e := range payload.Events {
			summary = append(summary, e.Type+"/"+e.Target)
		}
		t.Fatalf("expected reconciler.create for %s in events; got: %s", wantTarget, strings.Join(summary, ", "))
	}

	// Dashboard parity (SC-011): /ui/events renders the HTML page.
	html := harness.GetViaSocket(t, dir, token, "/ui/events")
	if !strings.Contains(html, "Audit Log") {
		t.Errorf("/ui/events did not render the Audit Log heading; body[:200]=%q", html[:min(200, len(html))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
