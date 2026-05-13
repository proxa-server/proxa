// Package version exposes build-time identity for the proxa and
// proxa-agent binaries. Values are overridden via `-ldflags`:
//
//	go build -ldflags "-X github.com/proxa-server/proxa/internal/version.Version=v0.0.1 \
//	                   -X github.com/proxa-server/proxa/internal/version.Commit=$(git rev-parse --short HEAD) \
//	                   -X github.com/proxa-server/proxa/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package version

// Version is the semantic version, set at build time. "dev" indicates
// an unreleased build from source.
var Version = "dev"

// Commit is the short Git SHA the binary was built from. "unknown"
// indicates the build did not pass -X overrides.
var Commit = "unknown"

// BuildDate is the UTC timestamp the binary was built at, in
// RFC 3339 format. "unknown" indicates the build did not pass -X overrides.
var BuildDate = "unknown"
