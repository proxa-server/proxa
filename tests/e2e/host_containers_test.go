//go:build e2e
// +build e2e

// Run with: make test-e2e
//
// Covers SC-001..SC-006 of specs/008-container-ui — the v0.4.4
// Containers dashboard surface. Boots a host container via `docker run`
// (no Proxa labels), then exercises the host-containers API + verifies
// the audit log.

package e2e

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

func TestSC_008_HostContainersListAndAct(t *testing.T) {
	harness.SCAttrs(t, "008-container-ui", "SC-001")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	// Boot one HOST container (no proxa labels).
	const hostName = "proxa-e2e-host-008"
	_ = exec.Command("docker", "rm", "-f", hostName).Run()
	if out, err := exec.Command("docker", "run", "-d", "--name", hostName,
		"--label", "io.example=v0.4.4-test",
		"nginx:alpine").CombinedOutput(); err != nil {
		t.Skipf("docker run failed (likely no daemon access): %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", hostName).Run() })

	// Give docker a moment to flip state to running.
	time.Sleep(2 * time.Second)

	token := harness.ReadToken(t, dir)

	// SC-001: list includes our host container with managed=false.
	body := harness.GetViaSocket(t, dir, token, "/api/v1/host-containers")
	var list struct {
		Containers []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Managed bool   `json:"managed"`
			State   string `json:"state"`
		} `json:"containers"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode list: %v\n%s", err, body)
	}
	var hostID string
	for _, c := range list.Containers {
		if strings.Contains(c.Name, hostName) {
			if c.Managed {
				t.Errorf("expected managed=false for host container %q, got true", c.Name)
			}
			hostID = c.ID
			break
		}
	}
	if hostID == "" {
		t.Fatalf("host container %q not in list; body=%s", hostName, body)
	}

	// SC-002: stop the host container via the API.
	if status := postAction(t, dir, token, hostID, "stop"); status != http.StatusNoContent {
		t.Errorf("stop: want 204, got %d", status)
	}
	time.Sleep(2 * time.Second)

	// SC-006: events log includes the user.container.stop event.
	evBody := harness.GetViaSocket(t, dir, token, "/api/v1/events?limit=100")
	if !strings.Contains(evBody, "user.container.stop") {
		t.Errorf("events log missing user.container.stop; body=%s", evBody)
	}
	if !strings.Contains(evBody, "container:"+hostID) {
		t.Errorf("events log missing target container:%s; body=%s", hostID, evBody)
	}

	// SC-002 (continued): start it back.
	if status := postAction(t, dir, token, hostID, "start"); status != http.StatusNoContent {
		t.Errorf("start: want 204, got %d", status)
	}

	// SC-005: remove while running with no force flag → 409.
	if status := deleteContainer(t, dir, token, hostID, false); status != http.StatusConflict {
		t.Errorf("remove-while-running without force: want 409, got %d", status)
	}

	// Stop + remove cleanly.
	_ = postAction(t, dir, token, hostID, "stop")
	time.Sleep(1500 * time.Millisecond)
	if status := deleteContainer(t, dir, token, hostID, false); status != http.StatusNoContent {
		t.Errorf("remove-stopped: want 204, got %d", status)
	}
}

func TestSC_008_HostContainersRejectManaged(t *testing.T) {
	harness.SCAttrs(t, "008-container-ui", "SC-003")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	// Deploy a managed service so we have a managed container to target.
	tomlPath := dir + "/svc.toml"
	tomlContent := `
name = "managed"
image = "nginx:alpine"
replicas = 1
[[expose]]
container = 80
host = 0
protocol = "http"
`
	if err := writeFile(tomlPath, tomlContent); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "proxa-default-managed-0").Run() })
	time.Sleep(7 * time.Second)

	token := harness.ReadToken(t, dir)
	body := harness.GetViaSocket(t, dir, token, "/api/v1/host-containers")
	var list struct {
		Containers []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Managed bool   `json:"managed"`
		} `json:"containers"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode list: %v\n%s", err, body)
	}
	var managedID string
	for _, c := range list.Containers {
		if c.Managed && strings.Contains(c.Name, "proxa-default-managed-0") {
			managedID = c.ID
			break
		}
	}
	if managedID == "" {
		t.Fatalf("managed container not surfaced in /api/v1/host-containers; body=%s", body)
	}
	if status := postAction(t, dir, token, managedID, "stop"); status != http.StatusConflict {
		t.Errorf("stop on managed container: want 409, got %d", status)
	}
}

func postAction(t *testing.T, dir, token, id, action string) int {
	t.Helper()
	return harness.PostViaSocketStatus(t, dir, token, "/api/v1/host-containers/"+id+"/"+action)
}

func deleteContainer(t *testing.T, dir, token, id string, force bool) int {
	t.Helper()
	path := "/api/v1/host-containers/" + id
	if force {
		path += "?force=true"
	}
	return harness.DeleteViaSocketStatus(t, dir, token, path)
}

func writeFile(p, content string) error {
	return harness.WriteFile(p, content)
}
