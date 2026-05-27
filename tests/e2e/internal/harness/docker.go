package harness

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// WaitForCount blocks until the running-container count for the named
// proxa-managed service reaches `want`, or the deadline passes.
func WaitForCount(t *testing.T, service string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "ps",
			"--filter", "label=proxa.managed=true",
			"--filter", "label=proxa.service="+service,
			"--format", "{{.Names}}").Output()
		if err == nil {
			n := len(strings.Fields(strings.TrimSpace(string(out))))
			if n == want {
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
	t.Errorf("service %q never reached %d running containers within %s", service, want, timeout)
}

// WaitForServiceStatus polls `proxa ps -j` until the named service
// shows the wanted status, or the deadline passes. Returns true on
// success.
func WaitForServiceStatus(t *testing.T, dir, service, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := RunProxa(t, dir, "ps", "-j")
		if err == nil {
			var doc map[string]any
			if json.Unmarshal([]byte(out), &doc) == nil {
				if findServiceStatus(doc, service) == want {
					return true
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func findServiceStatus(doc map[string]any, service string) string {
	projects, _ := doc["projects"].([]any)
	for _, p := range projects {
		proj, _ := p.(map[string]any)
		svcs, _ := proj["services"].([]any)
		for _, s := range svcs {
			svc, _ := s.(map[string]any)
			name, _ := svc["name"].(string)
			if name != service {
				continue
			}
			if status, ok := svc["status"].(string); ok {
				return strings.TrimSpace(status)
			}
		}
	}
	return ""
}

// SkipIfHTTPProbeUnreachable skips the test when the host cannot route
// directly to docker bridge IPs (Docker Desktop on macOS/Windows runs
// the bridge inside a VM).
func SkipIfHTTPProbeUnreachable(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return
	}
	t.Skip("HTTP probe e2e tests require a host that can route to docker bridge IPs " +
		"(direct from the host). Docker Desktop on " + runtime.GOOS + " runs the bridge " +
		"inside a VM, so the host cannot reach 172.17.0.x. Run this test on a Linux server, " +
		"or rely on the unit-level HTTP probe coverage in internal/probe/http_test.go.")
}

// DockerInspect returns the result of `docker inspect <container> --format <fmt>`.
func DockerInspect(t *testing.T, container, format string) string {
	t.Helper()
	out, err := exec.Command("docker", "inspect", container, "--format", format).CombinedOutput()
	if err != nil {
		t.Fatalf("docker inspect: %v\n%s", err, out)
	}
	return string(out)
}
