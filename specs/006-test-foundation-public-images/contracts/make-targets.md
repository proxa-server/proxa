# Contract — Makefile Targets

The Makefile matrix formalized in v0.4.2. Each target has a well-defined behavior, expected runtime, and exit code semantics.

## Target inventory

| Target | Behavior | Expected runtime | Exit codes |
|---|---|---|---|
| `make help` | List targets | <1s | 0 |
| `make build` | Build `proxa` + `proxa-agent` to `bin/` (existing) | ~10s | 0 success / 1 build fail |
| `make build-check` | Build + verify size within ±2% of `bench/binary-size-baseline.txt` | ~12s | 0 within / 1 build fail / 2 size out of budget (warn only in v0.4.2) |
| `make test` | Unit tests + race detector (default for CI + most contributors) | ~30s | 0 pass / 1 test fail |
| `make test-quick` | Unit tests WITHOUT race detector (fast dev loop) | ~5s | 0 pass / 1 test fail |
| `make test-integ` | `//go:build dockerd` tests against real Docker daemon | ~1-2 min | 0 pass / 1 fail / 2 daemon unavailable (skip) |
| `make test-e2e` | `//go:build e2e` tests against built binary | ~5-10 min | 0 pass / 1 fail / 2 daemon unavailable (skip) |
| `make bench` | Run all benchmarks in `./bench/...` with `-benchmem` | ~3-5 min | 0 always (benchmarks don't fail; results are data) |
| `make cover` | Generate `coverage.out` + HTML report + run `cmd/coverage-gate` | ~45s | 0 always in v0.4.2 (reporting-only) |
| `make lint` | `go vet ./...` + `go tool staticcheck ./...` (existing) | ~10s | 0 clean / 1 warnings |
| `make tidy` | `go mod tidy` (existing) | ~5s | 0 success |
| `make clean` | Remove `bin/`, `dist/`, `coverage.out`, `coverage.html` | <1s | 0 |
| `make release-dry` | NEW — local dry-run of release pipeline via Goreleaser snapshot | ~2-3 min | 0 success / 1 build fail / 2 docker buildx fail |
| `make release` | Tag-triggered ONLY (refuses to run if not on a vX.Y.Z tag) | ~5-10 min | 0 success / 1 not on tag / 2 build fail |

## Behavior detail

### `make test` (modified)

```make
test: ## Run all tests
	$(GO) test -race -count=1 ./...
```

Same as v0.4.1. Excludes `bench/` (the bench package has `// +build never` on its test files or uses a build tag).

### `make test-quick` (new)

```make
test-quick: ## Fast unit tests without race detector
	$(GO) test -count=1 ./...
```

Same packages, no `-race`. Useful when iterating on a single file.

### `make test-integ` (refined from existing)

```make
test-integ: ## //go:build dockerd integration tests (requires Docker daemon)
	$(GO) test -race -count=1 -tags dockerd ./...
```

Same as v0.4.1; no behavior change.

### `make test-e2e` (refined from existing)

```make
test-e2e: build ## E2E tests against built binary (requires Docker daemon)
	$(GO) test -count=1 -tags e2e ./tests/e2e/...
```

Same as v0.4.1; no behavior change.

### `make bench` (new)

```make
bench: ## Run benchmark suite (./bench/...)
	$(GO) test -bench=. -benchmem -run=^$ -count=3 ./bench/...
```

Skips test execution (`-run=^$`), runs benchmarks 3 times for stability. Output is plain `go test -bench` format. Benchmarks self-report domain metrics via `b.ReportMetric`.

### `make cover` (new)

```make
cover: ## Generate coverage report + per-package gate (reporting-only)
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./... 2>/dev/null
	$(GO) tool cover -html=coverage.out -o coverage.html
	$(GO) run ./cmd/coverage-gate -threshold 60 -allowlist .coverage-allowlist coverage.out
	@echo "→ coverage.html ready"
```

Excludes `bench/...` and `tests/...` from coverage (they're test code themselves).

### `make release-dry` (new)

```make
release-dry: ## Local dry-run of release pipeline (no publish)
	$(GO) tool goreleaser release --snapshot --skip=publish --clean
```

Validates the entire Goreleaser config end-to-end without pushing anything. Output lands in `dist/` for inspection.

### `make release` (new)

```make
release: ## Real release (intended for CI / git tag triggers only)
	@git describe --exact-match --tags HEAD >/dev/null 2>&1 || \
		(echo "error: not on a vX.Y.Z tag; use 'git tag vX.Y.Z' first" && exit 1)
	$(GO) tool goreleaser release --clean
```

Safety: refuses to run unless HEAD is on a tag. Operators almost never run this locally — it's the CI target.

## Variables

| Var | Default | Purpose |
|---|---|---|
| `GO` | `go` | Override Go binary (rare; useful for testing newer Go versions) |
| `STATICCHECK` | `honnef.co/go/tools/cmd/staticcheck@v0.7.0` | Legacy variable kept for external scripts; lint target uses `go tool staticcheck` |
| `VERSION` | from `git describe --tags --dirty --always` | Build-time version |
| `COMMIT` | from `git rev-parse --short HEAD` | Build-time commit |
| `BUILD_DATE` | UTC ISO 8601 from `date -u +%Y-%m-%dT%H:%M:%SZ` | Build-time date |
| `LDFLAGS` | `-X internal/version.Version=$(VERSION) ...` | Wraps version vars |

## Test coverage

No automated tests of the Makefile itself (Makefiles are notoriously hard to test). Validated via:
- Manual invocation of each target on a fresh clone (part of v0.4.2 acceptance)
- The release-dry e2e test (`docker_image_test.go`) implicitly validates the bench/cover/build targets work end-to-end
- CI workflow uses `make test`, `make lint`, `make build` — any breakage surfaces on first PR
