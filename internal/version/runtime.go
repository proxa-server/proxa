package version

import (
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// SystemInfo is the runtime introspection payload surfaced by the
// dashboard's System Info card, the /ui/system page, the
// GET /api/v1/system HTTP endpoint, and the `proxa system info` CLI.
//
// All fields are computed fresh on each call — there is no caching.
// The cost is microseconds.
//
// JSON tags use snake_case; this is the public wire format for the
// HTTP endpoint and the CLI's --json output. Adding fields is non-
// breaking; renaming or removing fields is a breaking change.
type SystemInfo struct {
	GoVersion        string   `json:"go_version"`
	Commit           string   `json:"commit"`
	BuildDate        string   `json:"build_date"`
	ProxaVersion     string   `json:"proxa_version"`
	GoExperiments    []string `json:"go_experiments"`
	GOMAXPROCS       int      `json:"gomaxprocs"`
	GOMAXPROCSSource string   `json:"gomaxprocs_source"`
	NumCPUHost       int      `json:"numcpu_host"`
	Distribution     string   `json:"distribution"`
}

// GOMAXPROCS source constants. Stable enum — values are part of the
// public API contract for the /api/v1/system endpoint and the CLI.
const (
	GOMAXPROCSSourceHost           = "host"
	GOMAXPROCSSourceContainerLimit = "container_limit"
	GOMAXPROCSSourceEnvOverride    = "env_override"
)

// Distribution channel constants. Stable enum — values are part of the
// public API contract. Added in v0.4.2 (006-test-foundation-public-images).
const (
	DistributionBinary  = "binary"  // host process (default fallback)
	DistributionDocker  = "docker"  // running inside a container
	DistributionUnknown = "unknown" // detection failed (rare)
)

// distributionCache holds the once-detected channel for the process
// lifetime. Detected lazily on first call to System() and never
// re-computed — Distribution can't change without restarting proxa.
var distributionCache string

// System returns a freshly-computed SystemInfo snapshot of the running
// process.
//
// GOMAXPROCS source detection (specs/005-modern-go/research.md R-003):
//
//	GOMAXPROCS env var set → "env_override"
//	gomaxprocs < numcpu_host → "container_limit" (Go 1.25+ container-aware)
//	otherwise → "host"
//
// Note: a host with N CPUs that is also limited to exactly N cores
// reports "host" rather than "container_limit". The observable effect
// is identical — Proxa uses all visible CPUs.
func System() SystemInfo {
	gomaxprocs := runtime.GOMAXPROCS(0)
	numcpu := runtime.NumCPU()

	source := GOMAXPROCSSourceHost
	if _, set := os.LookupEnv("GOMAXPROCS"); set {
		source = GOMAXPROCSSourceEnvOverride
	} else if gomaxprocs < numcpu {
		source = GOMAXPROCSSourceContainerLimit
	}

	return SystemInfo{
		GoVersion:        runtime.Version(),
		Commit:           Commit,
		BuildDate:        BuildDate,
		ProxaVersion:     Version,
		GoExperiments:    goExperiments(),
		GOMAXPROCS:       gomaxprocs,
		GOMAXPROCSSource: source,
		NumCPUHost:       numcpu,
		Distribution:     distribution(),
	}
}

// distribution detects how the running Proxa process was distributed.
// Cached on first call (the answer can't change without process
// restart). Detection priority (per
// specs/006-test-foundation-public-images/data-model.md):
//
//  1. PROXA_DISTRIBUTION env var (set by Dockerfile ENV) — authoritative
//  2. /.dockerenv file present OR PID == 1 — heuristic for "running in container"
//  3. fallback "binary"
//
// Returns one of DistributionBinary / DistributionDocker / DistributionUnknown.
func distribution() string {
	if distributionCache != "" {
		return distributionCache
	}
	if v := os.Getenv("PROXA_DISTRIBUTION"); v != "" {
		distributionCache = v
		return distributionCache
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		distributionCache = DistributionDocker
		return distributionCache
	}
	if os.Getpid() == 1 {
		// PID 1 strongly suggests container init (or PID-1 systemd, rare).
		distributionCache = DistributionDocker
		return distributionCache
	}
	distributionCache = DistributionBinary
	return distributionCache
}

// goExperiments returns the active GOEXPERIMENT flags as a slice.
// Empty slice (NOT nil) when none are active — the JSON encoding for
// the HTTP endpoint MUST be "[]" not "null", per the contract in
// specs/005-modern-go/contracts/system-info-api.md.
func goExperiments() []string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return []string{}
	}
	for _, s := range info.Settings {
		if s.Key != "GOEXPERIMENT" {
			continue
		}
		if s.Value == "" {
			return []string{}
		}
		parts := strings.Split(s.Value, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return []string{}
}
