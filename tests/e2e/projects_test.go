//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSC_006_MultiProjectIsolation verifies that two services with the
// same name in different projects coexist as independent containers.
func TestSC_006_MultiProjectIsolation(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	for _, p := range []string{"socio-do", "kut-do"} {
		// Project must exist before upserting a service in it (see handlers.go).
		if out, err := runProxa(t, dir, "ps"); err != nil {
			t.Fatalf("ps before project create: %v\n%s", err, out)
		}
		// Create project via API to avoid needing a `proxa project` cmd in v0.0.
		createProjectViaCurl(t, dir, p)

		tomlPath := filepath.Join(dir, p+".toml")
		toml := "project = \"" + p + "\"\nname = \"web\"\nimage = \"traefik/whoami:latest\"\nreplicas = 1\n"
		if err := os.WriteFile(tomlPath, []byte(toml), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
			t.Fatalf("up %s: %v\n%s", p, err, out)
		}
	}

	// Wait for both to converge.
	for _, p := range []string{"socio-do", "kut-do"} {
		deadline := time.Now().Add(15 * time.Second)
		got := false
		for time.Now().Before(deadline) {
			out, _ := exec.Command("docker", "ps",
				"--filter", "label=proxa.project="+p,
				"--filter", "label=proxa.service=web",
				"--format", "{{.Names}}").Output()
			if strings.TrimSpace(string(out)) != "" {
				got = true
				break
			}
			time.Sleep(1 * time.Second)
		}
		if !got {
			t.Errorf("project %q web container never appeared", p)
		}
	}

	t.Cleanup(func() {
		for _, p := range []string{"socio-do", "kut-do"} {
			_ = exec.Command("docker", "rm", "-f", "proxa-"+p+"-web-0").Run()
		}
	})
}

// createProjectViaCurl POSTs to /api/v1/projects via the unix socket.
// In v0.0 there's no `proxa project create` command.
func createProjectViaCurl(t *testing.T, dir, name string) {
	t.Helper()
	tokenBytes, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	sock := socketPath(t, dir)

	cmd := exec.Command("curl", "-sS", "--unix-socket", sock,
		"-H", "Authorization: Bearer "+token,
		"-H", "Content-Type: application/json",
		"-d", `{"name":"`+name+`"}`,
		"http://x/api/v1/projects")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create project %q: %v\n%s", name, err, out)
	}
}
