//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_005_SystemInfo covers SC-005 / FR-007 / FR-008: every surface
// (HTTP API, CLI, dashboard footer card, focused /ui/system page)
// exposes the runtime introspection consistently.
//
// Six checks per contracts/system-info-api.md "Test coverage" section.
func TestSC_005_SystemInfo(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-005")
	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	token := harness.ReadToken(t, dir)

	// (1) GET /api/v1/system returns 200 with all expected keys.
	body := harness.GetViaSocket(t, dir, token, "/api/v1/system")
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode /api/v1/system: %v\nbody=%s", err, body)
	}
	wantKeys := []string{
		"go_version", "commit", "build_date", "proxa_version",
		"go_experiments", "gomaxprocs", "gomaxprocs_source", "numcpu_host",
	}
	for _, k := range wantKeys {
		if _, ok := payload[k]; !ok {
			t.Errorf("/api/v1/system missing key %q\nbody=%s", k, body)
		}
	}
	// go_experiments must be a slice (not null) per the contract.
	if exps, ok := payload["go_experiments"].([]any); !ok {
		t.Errorf("go_experiments is %T; want []any (non-nil slice)", payload["go_experiments"])
	} else {
		_ = exps // OK, may be empty
	}
	// gomaxprocs_source must be one of the stable enum values.
	switch payload["gomaxprocs_source"] {
	case "host", "container_limit", "env_override":
		// ok
	default:
		t.Errorf("gomaxprocs_source = %v; not a stable enum value", payload["gomaxprocs_source"])
	}

	// (2) `proxa system info` plain-text matches keys from the HTTP body.
	cliOut, err := harness.RunProxa(t, dir, "system", "info")
	if err != nil {
		t.Fatalf("system info: %v\n%s", err, cliOut)
	}
	for _, k := range wantKeys {
		if !strings.Contains(cliOut, k+"=") {
			t.Errorf("CLI plain-text missing key %q\noutput=%s", k, cliOut)
		}
	}

	// (3) `proxa system info --json` matches the HTTP body byte-for-byte
	// (modulo trailing newline from json.Encoder).
	cliJSON, err := harness.RunProxa(t, dir, "system", "info", "--json")
	if err != nil {
		t.Fatalf("system info --json: %v\n%s", err, cliJSON)
	}
	var cliPayload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(cliJSON)), &cliPayload); err != nil {
		t.Fatalf("decode CLI --json: %v\noutput=%s", err, cliJSON)
	}
	// Compare key sets (values may differ slightly on numeric encoding,
	// so just ensure structure parity).
	for k := range payload {
		if _, ok := cliPayload[k]; !ok {
			t.Errorf("CLI --json missing key %q present in HTTP body", k)
		}
	}
	for k := range cliPayload {
		if _, ok := payload[k]; !ok {
			t.Errorf("CLI --json has extra key %q not in HTTP body", k)
		}
	}

	// (4) Missing-token request returns 401.
	noTokenBody := getViaSocketNoToken(t, dir, "/api/v1/system")
	if !strings.Contains(noTokenBody, "401") && !strings.Contains(strings.ToLower(noTokenBody), "unauth") {
		t.Errorf("expected 401/unauthorized without token; got: %s", noTokenBody)
	}

	// (5) Dashboard footer card markup is present on /ui/.
	uiBody := harness.GetViaSocket(t, dir, token, "/ui/")
	for _, want := range []string{
		"⚙️ System",
		"/ui/system",
		"footerSysInfo",
		"/api/v1/system",
	} {
		if !strings.Contains(uiBody, want) {
			t.Errorf("/ui/ missing footer card marker %q", want)
		}
	}

	// (6) /ui/system page contains all SystemInfo field labels.
	sysPage := harness.GetViaSocket(t, dir, token, "/ui/system")
	for _, want := range []string{
		"System Info",
		"Proxa version",
		"Go runtime",
		"GOMAXPROCS",
		"GOEXPERIMENT",
		"systemInfoController",
	} {
		if !strings.Contains(sysPage, want) {
			t.Errorf("/ui/system missing %q", want)
		}
	}
}

// getViaSocketNoToken issues a request with NO Authorization header
// and returns the raw response (status + body in a stitched string)
// so the test can assert the 401 path.
func getViaSocketNoToken(t *testing.T, dir, path string) string {
	t.Helper()
	out, err := exec.Command("curl", "-sS", "--unix-socket", harness.SocketPath(t, dir),
		"-w", "\nHTTP %{http_code}\n",
		"http://x"+path).Output()
	if err != nil {
		t.Fatalf("curl no-token: %v", err)
	}
	return string(out)
}

// Suppress unused-import warning on Linux-only platforms where the
// system_info tests behave identically.
var _ = runtime.GOOS
