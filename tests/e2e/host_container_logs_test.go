//go:build e2e
// +build e2e

// Run with: make test-e2e
//
// Covers SC-002 / SC-003 of specs/009-probe-events-host-logs.

package e2e

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

func TestSC_009_HostContainerLogsAPI(t *testing.T) {
	harness.SCAttrs(t, "009-probe-events-host-logs", "SC-002")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	const hostName = "proxa-e2e-loghost-009"
	_ = exec.Command("docker", "rm", "-f", hostName).Run()
	// Run an alpine container that prints a sentinel + exits — its
	// stdout shows up in logs immediately.
	if out, err := exec.Command("docker", "run", "-d", "--name", hostName,
		"alpine:latest", "sh", "-c", "for i in 1 2 3; do echo PROXA_E2E_SENTINEL_$i; done; sleep 30").CombinedOutput(); err != nil {
		t.Skipf("docker run failed: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", hostName).Run() })

	// Give docker a moment to capture the stdout lines.
	time.Sleep(2 * time.Second)

	// Resolve container id via the list endpoint.
	token := harness.ReadToken(t, dir)
	body := harness.GetViaSocket(t, dir, token, "/api/v1/host-containers")
	var list struct {
		Containers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"containers"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode list: %v\n%s", err, body)
	}
	var id string
	for _, c := range list.Containers {
		if strings.Contains(c.Name, hostName) {
			id = c.ID
			break
		}
	}
	if id == "" {
		t.Fatalf("host container %s not in list", hostName)
	}

	// SC-002: GET /api/v1/host-containers/{id}/logs returns the sentinel.
	logs := harness.GetViaSocket(t, dir, token, "/api/v1/host-containers/"+id+"/logs?tail=10")
	for i := 1; i <= 3; i++ {
		want := "PROXA_E2E_SENTINEL_" + string(rune('0'+i))
		if !strings.Contains(logs, want) {
			t.Errorf("host logs missing %q; body=%q", want, logs)
		}
	}

	// SC-003: /ui/logs/host/{id} renders the page.
	html := harness.GetViaSocket(t, dir, token, "/ui/logs/host/"+id)
	if !strings.Contains(html, "Container Logs") || !strings.Contains(html, id) {
		t.Errorf("/ui/logs/host page did not render container header; body[:200]=%q", html[:min(200, len(html))])
	}
}
