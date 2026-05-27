# Research — 006-test-foundation-public-images (v0.4.2)

Phase 0 decisions that resolve the six open questions in `plan.md`. Each decision includes rationale + alternatives considered so future maintainers can re-litigate from the same footing.

## R-001 — Release tool choice

**Question**: Use Goreleaser (existing partial config at `.goreleaser.yml`) or replace with a hand-rolled GitHub Actions matrix build?

**Existing state**: `.goreleaser.yml` is already present with the builds matrix configured for proxa + proxa-agent across linux/darwin × amd64/arm64. Archive + checksum + snapshot config also present. **Missing**: `dockers` + `docker_manifests` blocks for image building, and the GitHub Actions workflow that triggers Goreleaser on tag push.

**Decision**: **Extend the existing Goreleaser config.** Add `dockers` + `docker_manifests` blocks for multi-arch image build/push. Add `.github/workflows/release.yml` that runs `goreleaser release --clean` on tag push.

**Rationale**:
- Zero migration cost — config already exists and has been reviewed.
- Goreleaser is the de-facto Go release tool; ~100k+ Go projects use it. Easy for new contributors.
- Hand-rolled Actions matrix would be ~3x more YAML for identical behavior.
- Goreleaser handles cross-compilation, archive packing, checksum generation, GitHub Release upload, AND Docker buildx all from one config file. Splitting across multiple workflows duplicates the build matrix.

**Alternatives considered**:
- **Hand-rolled Actions matrix** — rejected per above.
- **Replace Goreleaser with `go build` + `docker buildx` + `gh release create` in a shell script** — rejected: re-implements 1500 LOC of Goreleaser logic poorly.
- **ko (image-only)** — rejected: doesn't handle binary releases, would still need Goreleaser for tarballs.

**Test footprint**: Goreleaser dry-run target in Makefile (`make release-dry`) runs `go tool goreleaser release --snapshot --skip=publish --clean`. The e2e test `docker_image_test.go` builds a local image via `go tool goreleaser` and verifies it boots.

## R-002 — Goreleaser via Go tool directive

**Question**: How do contributors invoke Goreleaser without a global install? Follow the v0.4.1 staticcheck precedent (Go 1.24 `tool` directive)?

**License check**: Goreleaser is **MIT** (github.com/goreleaser/goreleaser/v2/LICENSE.md, verified). Compatible with §IX allow-list. Will be added to `docs/licenses.md` refresh log.

**Decision**: Add `tool github.com/goreleaser/goreleaser/v2/cmd/goreleaser` to `go.mod`. Makefile invokes via `go tool goreleaser`. No global install required for contributors or CI.

**Rationale**:
- Same pattern as v0.4.1 staticcheck (`tool honnef.co/go/tools/cmd/staticcheck` in `go.mod`).
- Zero contributor friction: clone repo, `make release-dry` works.
- Version pinned in `go.mod` — no version skew between contributors.
- CI uses the same invocation (`go tool goreleaser release --clean`) — no install step in release.yml.

**Invocation pattern**:
```sh
go get -tool github.com/goreleaser/goreleaser/v2/cmd/goreleaser@v2.4.0
# Adds the tool directive + downloads to module cache.
```

In Makefile:
```make
release-dry: ## Local dry-run of the release pipeline (no publish)
	$(GO) tool goreleaser release --snapshot --skip=publish --clean
```

**Alternatives considered**:
- **Global `brew install goreleaser`** — rejected: contributors on Linux don't get brew, and version drifts between machines.
- **Docker container** — rejected: extra layer, slower for dev loop.
- **GitHub Action wrapper (`goreleaser/goreleaser-action`)** — rejected: hides the invocation from local dev, breaks the "same command locally and in CI" principle.

## R-003 — install.sh hosting

**Question**: Where to host the `curl-pipe-sh` install script so the URL is stable + zero new infra?

**Options**:
- **(A) `docs/install/install.sh` served via GitHub Pages from main branch.** URL: `https://proxa-server.github.io/proxa/install/install.sh`.
- **(B) `gh-pages` orphan branch with `install.sh` at root.** URL: `https://proxa-server.github.io/proxa/install.sh`.
- **(C) Custom CNAME `get.proxa.sh`** — domain registration + DNS + GH Pages CNAME setup.

**Decision**: **Option (A) — `docs/install/install.sh` served from main via GitHub Pages.**

**Rationale**:
- Single-branch story — no `gh-pages` orphan branch to maintain. install.sh evolves with the rest of the repo on main, no manual sync needed.
- File lives alongside other docs; easy to discover for contributors.
- The "real" install.sh is at repo root `install.sh` (for direct invocation from a fresh checkout); `docs/install/install.sh` is a hardlink/symlink during build OR a workflow copy step that mirrors it. Decision in tasks: prefer a file-copy step in CI over symlinks (cross-platform clarity).
- Zero new infrastructure required. GitHub Pages from main is a single repo setting.
- Custom CNAME (option C) is a future polish — documented in `operations.md` as upgrade path when project gets a domain.

**Published URL pattern**:
```
https://proxa-server.github.io/proxa/install/install.sh
```

GitHub Pages auto-publishes within ~30 seconds of a push to main.

**Operator install command** (becomes the canonical one-liner):
```sh
curl -fsSL https://proxa-server.github.io/proxa/install/install.sh | sh
```

**Alternatives considered**:
- **gh-pages orphan branch** — rejected: extra branch management, manual sync, deployment lag.
- **Custom CNAME (get.proxa.sh)** — rejected for v0.4.2 (no domain yet). Documented as upgrade path.
- **Host on the binary release itself** — circular dependency (need install.sh to download binaries, can't get install.sh from the binary release).

## R-004 — Synctest scope and httptest interaction

**Question**: Exact migration targets locked. Does `testing/synctest` interact correctly with `httptest.Server` in `internal/probe/http_test.go`?

**Pre-survey** (executed during plan generation):
```
internal/reconciler/reconciler_test.go:162  time.Sleep(100 * time.Millisecond)
internal/reconciler/reconciler_test.go:168  time.Sleep(100 * time.Millisecond)
internal/probe/manager_test.go:89           time.Sleep(10 * time.Second)
internal/probe/manager_test.go:123          time.Sleep(30 * time.Second)
internal/probe/http_test.go:69              time.Sleep(500 * time.Millisecond)
internal/probe/http_test.go:87              time.Sleep(2 * time.Second)
```

6 sleeps total. The reconciler + manager sleeps are pure goroutine-coordination waits (the test is waiting for a ticker to fire). These are EASY synctest migrations — wrap the test body in `synctest.Run(func() { ... })`, replace sleep with `synctest.Wait()` after the operation that causes the goroutine to make progress.

The `internal/probe/http_test.go` sleeps are different — they're inside `httptest.Server` handlers (delaying the server's response to simulate slow/cancellable requests):

```go
// Line 69 context:
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
    time.Sleep(500 * time.Millisecond)   // ← inside server handler
    w.WriteHeader(http.StatusOK)
}))
```

**Decision**: Migrate the 4 reconciler+manager sleeps to synctest. **Do NOT** migrate the http_test.go sleeps — they are inside `httptest.Server` handler goroutines that synctest cannot bubble (httptest spawns its own goroutines outside the test's synctest bubble). These tests stay using real `time.Sleep` for now; they're already short (500ms + 2s) and not flaky.

**Rationale**:
- 4 of 6 sleeps migrate cleanly with measurable wall-time improvement (~40s saved per test run if both manager_test.go sleeps had been real waits — synctest makes them effectively zero).
- 2 http_test.go sleeps stay; documented in test godoc as "intentionally real wait — simulates server delay".
- Migration scope honestly bounded; we don't pretend synctest is universal.

**Test pattern** (for the migrated 4):
```go
import "testing/synctest"

func TestReconciler_TickInterval_WithSynctest(t *testing.T) {
    synctest.Run(func() {
        r := New(...)
        go r.Run(ctx)
        // Trigger something.
        synctest.Wait()  // Wait for all goroutines to block; virtual time advances.
        // Assert reconciler state.
    })
}
```

**Alternatives considered**:
- **Mock `time.NewTicker`** — rejected: requires interface abstraction in production code that adds nothing to runtime.
- **Use `clock` package (3rd-party)** — rejected: §V prohibits new prod deps; would also require production-code refactor.
- **Migrate http_test.go via channel-based server** — possible but adds complexity for marginal gain; deferred.

## R-005 — Coverage script language

**Question**: Implement the `make cover` gate as a shell script or a small Go binary?

**Decision**: **Small Go binary at `cmd/coverage-gate/main.go` (~80 LOC).**

**Rationale**:
- Cross-platform — macOS uses BSD `awk`, Linux uses GNU `awk`. Shell scripts that parse `coverage.out` have subtle portability issues.
- Testable — `cmd/coverage-gate/main_test.go` covers parsing edge cases.
- Reusable — v0.4.3 can promote to hard-fail gate by changing one constant + tests.
- In-tree pattern matches existing precedent (`cmd/proxa`, `cmd/proxa-agent`).
- 80 LOC is the upper bound; likely shorter.

**Binary contract** (full detail in `contracts/coverage-gate.md`):
```
cmd/coverage-gate read coverage.out [--allowlist .coverage-allowlist] [--threshold 60]
  → stdout: per-package table sorted ascending by coverage %, packages below threshold marked with "⚠" prefix
  → exit code: always 0 in v0.4.2 (reporting-only); v0.4.3+ may flip to non-zero on threshold violation
```

**Alternatives considered**:
- **Shell + awk** — rejected: portability + testability concerns.
- **Use existing `go-test-coverage` (kyoh86/richgo) or similar 3rd-party tool** — rejected: violates §V (zero new prod deps); also adds tool-directive surface.
- **Inline in Makefile (large recipe with go list piping)** — rejected: too hard to read, untestable.

## R-006 — Bench layout + Linux-only memory bench

**Question**: Where does `bench/` live? How does `bench_idle_memory_test.go` handle macOS's different process memory accounting?

**Decision**:
- **`bench/` as top-level directory** (sibling to `internal/`, `cmd/`, `tests/`). Has its own package `bench` (not `bench_test` — the bench files need access to harness helpers).
- **Excluded from default test runs.** `make test` does NOT run `./bench/...`. `make bench` explicitly targets `./bench/...` with `-bench=. -benchmem -run=^$`.
- **Excluded from coverage gate.** `cmd/coverage-gate` skips `bench/...` paths (also `cmd/coverage-gate/...` itself and `tests/...`).
- **`bench_idle_memory_test.go` uses build tags**: `//go:build linux` for the procfs-based path that reads `/proc/<pid>/status` VmRSS. macOS gets a separate `bench_idle_memory_macos_test.go` with `//go:build darwin` that uses `runtime/debug.ReadMemStats` for a portable approximation. Both report via `b.ReportMetric(rss_mb, "MB-rss")` so the units are consistent across platforms.

**Rationale**:
- Top-level `bench/` is the Go community convention (see hashicorp/raft, etcd-io/etcd, google/pprof — all have top-level `bench/` or `benchmarks/`).
- Excluding from `make test` keeps the unit test suite fast (bench can take minutes).
- Excluding from coverage gate prevents the gate from chasing bench-test coverage (bench files are benchmarks, not unit tests; coverage is the wrong metric).
- Platform-specific memory measurement is honest: macOS process memory is virtual + dirty + compressed; comparing to Linux VmRSS is apples-to-oranges. Better to report 2 numbers explicitly than 1 misleading number.

**Bench file shape** (example, full in tasks):
```go
//go:build linux

package bench

func BenchmarkIdleMemoryRSS_Linux(b *testing.B) {
    cmd := exec.Command(harness.ProxaBinary(b), "server", "--data-dir", b.TempDir())
    if err := cmd.Start(); err != nil { b.Fatal(err) }
    defer cmd.Process.Kill()
    time.Sleep(10 * time.Second)  // settle
    rss := readVmRSSKB(cmd.Process.Pid)
    b.ReportMetric(float64(rss)/1024, "MB-rss")
}
```

**Alternatives considered**:
- **Bench under `internal/` per-package** — rejected: scatters bench surface, harder to run holistically.
- **Bench under `tests/bench/`** — rejected: confuses "tests" (correctness) with "bench" (performance).
- **Single `bench_idle_memory_test.go` with runtime branching** — rejected: hides the platform difference; explicit build tags are clearer.

## Cross-cutting

All 6 decisions converge on the same principles:
- **Reuse what works** (Goreleaser config, staticcheck tool-directive pattern, existing harness helpers).
- **Honest scope** (synctest doesn't apply to httptest; bench has different concerns on macOS; coverage gate is reporting-only this release).
- **Future-compatible** (v0.4.3 can flip coverage to hard-fail; v0.5 inherits agent Dockerfile; v0.7+ can flip release pipeline to add cosign signing).
