package version_test

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/proxa-server/proxa/internal/version"
)

func TestSystem_AlwaysPopulatesAllFields(t *testing.T) {
	info := version.System()

	if info.GoVersion == "" {
		t.Errorf("GoVersion empty")
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("GoVersion %q does not start with \"go\"", info.GoVersion)
	}
	if info.GOMAXPROCS <= 0 {
		t.Errorf("GOMAXPROCS = %d, want > 0", info.GOMAXPROCS)
	}
	if info.NumCPUHost <= 0 {
		t.Errorf("NumCPUHost = %d, want > 0", info.NumCPUHost)
	}
	switch info.GOMAXPROCSSource {
	case version.GOMAXPROCSSourceHost,
		version.GOMAXPROCSSourceContainerLimit,
		version.GOMAXPROCSSourceEnvOverride:
		// ok
	default:
		t.Errorf("GOMAXPROCSSource %q is not a valid enum value", info.GOMAXPROCSSource)
	}
	if info.ProxaVersion == "" {
		t.Errorf("ProxaVersion empty")
	}
	// GoExperiments must be non-nil even when empty — the JSON encoding
	// MUST be "[]" not "null" per the contract.
	if info.GoExperiments == nil {
		t.Errorf("GoExperiments is nil; want non-nil empty slice")
	}
}

func TestSystem_GoExperimentsEncodesAsEmptyArray(t *testing.T) {
	info := version.System()
	// Force the empty case for the encoding assertion.
	info.GoExperiments = []string{}

	b, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"go_experiments":[]`) {
		t.Errorf("expected JSON to contain \"go_experiments\":[]; got: %s", b)
	}
}

func TestSystem_GOMAXPROCSSource_EnvOverride(t *testing.T) {
	t.Setenv("GOMAXPROCS", "1")
	prev := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	info := version.System()
	if info.GOMAXPROCSSource != version.GOMAXPROCSSourceEnvOverride {
		t.Errorf("GOMAXPROCSSource = %q, want %q",
			info.GOMAXPROCSSource, version.GOMAXPROCSSourceEnvOverride)
	}
}

func TestSystem_GOMAXPROCSSource_HostMatch(t *testing.T) {
	withUnsetEnv(t, "GOMAXPROCS")
	prev := runtime.GOMAXPROCS(runtime.NumCPU())
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	info := version.System()
	if info.GOMAXPROCS != info.NumCPUHost {
		t.Skipf("test precondition not met: GOMAXPROCS=%d, NumCPUHost=%d (container-aware?)",
			info.GOMAXPROCS, info.NumCPUHost)
	}
	if info.GOMAXPROCSSource != version.GOMAXPROCSSourceHost {
		t.Errorf("GOMAXPROCSSource = %q, want %q",
			info.GOMAXPROCSSource, version.GOMAXPROCSSourceHost)
	}
}

func TestSystem_GOMAXPROCSSource_ContainerLimitHeuristic(t *testing.T) {
	// We can't directly simulate container_limit without actually running
	// inside a cgroup-limited container. Instead, verify the heuristic
	// classification logic by manipulating GOMAXPROCS to be less than
	// NumCPU while the env var is unset.
	withUnsetEnv(t, "GOMAXPROCS")
	if runtime.NumCPU() < 2 {
		t.Skip("host has <2 CPUs; can't simulate container_limit lower than host")
	}
	prev := runtime.GOMAXPROCS(1) // pretend the runtime auto-adjusted to 1
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	info := version.System()
	if info.GOMAXPROCSSource != version.GOMAXPROCSSourceContainerLimit {
		t.Errorf("GOMAXPROCSSource = %q, want %q",
			info.GOMAXPROCSSource, version.GOMAXPROCSSourceContainerLimit)
	}
	if info.GOMAXPROCS != 1 {
		t.Errorf("GOMAXPROCS = %d, want 1 (we forced it)", info.GOMAXPROCS)
	}
}

// withUnsetEnv unsets key for the duration of t and restores it via
// t.Cleanup. Used by tests that need to confirm GOMAXPROCS source
// detection when the env var is genuinely absent.
func withUnsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
