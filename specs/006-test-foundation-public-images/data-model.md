# Data Model — 006-test-foundation-public-images (v0.4.2)

This release introduces **no persistent entities**. It adds one in-memory field on an existing payload, two new repo-tracked files (a baseline number and an allowlist), and locks the shape of two new on-disk artifacts (container images and the install.sh script).

## In-memory additions

### SystemInfo.Distribution *(new field on existing payload)*

The dashboard's System Info card (shipped in v0.4.1) gets one more field. Detected at process start; immutable for the process lifetime.

| Field | Type | Detection logic | JSON tag |
|---|---|---|---|
| `Distribution` | string enum | see below | `distribution` |

Enum values:
- `"binary"` — Proxa is running as a host process (default fallback)
- `"docker"` — Proxa is running inside a container (detected by presence of `/.dockerenv` OR `os.Getpid() == 1`)
- `"unknown"` — neither detection succeeded (rare; future: kubernetes pod, systemd-nspawn, FreeBSD jail)

Detection happens once at server startup, stored in package-level var, returned on every `SystemInfo()` call. **No file I/O on the request path.**

**Backwards compatibility**: adding a field to the JSON struct is non-breaking. v0.4.1 clients reading the payload simply ignore the new field. CLI `proxa system info` automatically picks it up (it dumps the map).

**Test coverage** (added to `internal/version/runtime_test.go`):
- `TestSystem_Distribution_DockerEnvFile` — `os.WriteFile("/.dockerenv", nil, ...)` in t.TempDir() (with detector parameterized to check that path) → `"docker"`
- `TestSystem_Distribution_NotInContainer` → `"binary"`
- `TestSystem_Distribution_DefaultUnknownOnError` — detector returns error → `"unknown"`
- `TestSystem_DistributionRoundtripJSON` — encode + decode preserves the value

## Repo-tracked file additions

### bench/binary-size-baseline.txt

Single integer (byte count) on a single line. Committed to repo. Used by `make build-check` (or directly by `make bench`) to detect binary size regression.

Format:
```
27450050
```

That's the v0.4.1 binary size (recorded 2026-05-21 from the v0.4.1 build artifact). The ±2% budget in FR-016 is computed against this.

Update policy:
- When binary size intentionally changes (new feature adds bytes), the baseline is updated in the SAME commit that adds the feature.
- When binary size unexpectedly grows >2% without an accompanying baseline update, CI flags it (reporting-only in v0.4.2; hard-fail deferred to a later release).

### .coverage-allowlist

Newline-delimited list of Go import paths exempted from the 60% coverage gate. Comment-friendly (`#` prefix lines ignored).

Format:
```
# Packages exempted from the coverage gate.
# One import path per line. Comments start with #.
# Add a brief rationale next to each entry.

# Web template package — coverage is meaningless for HTML files.
github.com/proxa-server/proxa/internal/web

# Wire-format type definitions — no behavior to test.
github.com/proxa-server/proxa/pkg/types
```

Initial seed: the two paths above. Future additions require justification in the comment line.

## On-disk artifact shapes (NEW for this release)

These aren't entities in the runtime sense, but they ARE artifacts the release pipeline produces. Their shapes are versioned contracts.

### Container images (GHCR)

**Image path**: `ghcr.io/proxa-server/proxa:<tag>` and `ghcr.io/proxa-server/proxa-agent:<tag>`

**Tag scheme**:
- `vX.Y.Z` — specific version (e.g., `v0.4.2`)
- `vX.Y` — minor stream (e.g., `v0.4`)
- `latest` — most recent stable release (auto-updated)

**Manifest**: multi-arch via `docker manifest`. Linux only for v0.4.2.
- `linux/amd64`
- `linux/arm64`

**Labels** (OCI standard, populated by Goreleaser from Git context):
- `org.opencontainers.image.source` = `https://github.com/proxa-server/proxa`
- `org.opencontainers.image.version` = `{{.Version}}` (e.g., `v0.4.2`)
- `org.opencontainers.image.revision` = `{{.FullCommit}}`
- `org.opencontainers.image.licenses` = `Apache-2.0`
- `org.opencontainers.image.title` = `proxa` (or `proxa-agent`)
- `org.opencontainers.image.description` = short product description

**Size budget** (soft; documented in operations.md):
- `proxa`: < 80 MB compressed
- `proxa-agent`: < 40 MB compressed (stub binary is smaller)

### GitHub Release artifacts (per tag)

For each release tag `v*`, the pipeline attaches:
- `proxa_<version>_<os>_<arch>.tar.gz` — binary archive (per OS/arch in matrix: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- `proxa-agent_<version>_<os>_<arch>.tar.gz` — same shape for the agent binary
- `checksums.txt` — SHA-256 of every artifact in the release, format `<hex> <filename>` one per line (Goreleaser default)
- `install.sh` — copy of the canonical installer (so operators can `curl <release-url>/install.sh | sh` if Pages is unreachable)

### install.sh contract

Detailed in `contracts/install-sh.md`. The shape is:
- POSIX shell (tested with `dash`)
- Reads env vars: `INSTALL_VERSION` (default: latest), `INSTALL_DIR` (default: `/usr/local/bin`)
- Exit codes: 0 success / 1 unsupported platform / 2 download failed / 3 checksum mismatch / 4 install dir not writable
- Stdout: progress lines (machine-parseable: one event per line)
- Stderr: errors only

## Migration / compatibility notes

- **No on-disk format changes**: SQLite schema unchanged, secret format unchanged, token format unchanged.
- **No wire-format breaking changes**: SystemInfo JSON gets a new field; old clients ignore it.
- **Container images are NEW artifacts**: no migration concern (operators opt-in by switching to docker run).
- **install.sh is NEW**: operators continue using existing distribution methods (manual download) if they don't want it.
- **Goreleaser tool directive is build-time only**: production binary closure is unchanged. `go mod tidy` round-trip on `go.mod` changes: only the tool directive line + its transitive go.sum entries. NOT linked into proxa binary.

## State transitions

None. v0.4.2 is stateless w.r.t. its own data model. The Distribution field is computed once and stored; the baseline file is human-edited; the allowlist is human-edited.
