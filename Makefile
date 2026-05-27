# Proxa — make targets
#
# CGO is disabled everywhere (constitution §IV). Build-time identity is
# injected via -ldflags into internal/version. Lint + release tooling are
# pinned via `tool` directives in go.mod (v0.4.1+) so contributors do
# not need global installs.

GO              ?= go
STATICCHECK     := honnef.co/go/tools/cmd/staticcheck@v0.7.0   # legacy: see `go tool` directive in go.mod (v0.4.1+)
VERSION         := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT          := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS         := -X github.com/proxa-server/proxa/internal/version.Version=$(VERSION) \
                   -X github.com/proxa-server/proxa/internal/version.Commit=$(COMMIT) \
                   -X github.com/proxa-server/proxa/internal/version.BuildDate=$(BUILD_DATE)

# Binary size budget (v0.4.2+): bin/proxa must stay within ±2% of this baseline.
# Update bench/binary-size-baseline.txt in the SAME commit that intentionally
# changes binary size (e.g., a new feature adds bytes).
BIN_SIZE_BASELINE := $(shell cat bench/binary-size-baseline.txt 2>/dev/null || echo 0)

.PHONY: build build-check test test-quick test-integ test-integration test-e2e bench cover lint clean tidy mirror-install release-dry release help

help: ## List targets
	@awk 'BEGIN{FS=":.*##"; printf "Targets:\n"} /^[a-zA-Z_-]+:.*##/ {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build proxa and proxa-agent into bin/
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/proxa ./cmd/proxa
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/proxa-agent ./cmd/proxa-agent

build-check: build ## Build + report binary size delta vs bench/binary-size-baseline.txt (warn if outside ±2%)
	@actual=$$(stat -f%z bin/proxa 2>/dev/null || stat -c%s bin/proxa); \
	baseline=$(BIN_SIZE_BASELINE); \
	if [ "$$baseline" -eq 0 ]; then \
		echo "warn: bench/binary-size-baseline.txt missing or zero; skipping size check"; \
		exit 0; \
	fi; \
	delta_bytes=$$((actual - baseline)); \
	delta_pct_x100=$$((delta_bytes * 10000 / baseline)); \
	printf "bin/proxa size: %d bytes (baseline %d, delta %+d bytes = %+d.%02d%%)\n" \
		"$$actual" "$$baseline" "$$delta_bytes" \
		$$((delta_pct_x100 / 100)) $$(( (delta_pct_x100 < 0 ? -delta_pct_x100 : delta_pct_x100) % 100 )); \
	if [ "$$delta_pct_x100" -lt -200 ] || [ "$$delta_pct_x100" -gt 200 ]; then \
		echo "warn: binary size out of ±2% budget — update bench/binary-size-baseline.txt in the same commit if intentional"; \
	fi

test: ## Run all unit tests (race detector enabled; default for CI + reviewers)
	$(GO) test -race -count=1 ./...

test-quick: ## Fast unit tests WITHOUT race detector (dev loop)
	$(GO) test -count=1 ./...

test-integ: ## //go:build dockerd integration tests (requires Docker daemon)
	$(GO) test -race -count=1 -tags dockerd ./...

# Alias for backward compatibility with prior Makefile.
test-integration: test-integ ## Alias for test-integ

test-e2e: build ## E2E tests against the built binary (requires Docker daemon)
	$(GO) test -count=1 -tags e2e ./tests/e2e/...

bench: ## Run the benchmark suite (./bench + per-package benches) with -benchmem
	$(GO) test -bench=. -benchmem -run=^$$ -count=3 \
		./bench/... \
		./internal/reconciler/... \
		./internal/probe/... \
		./internal/ingress/...

cover: ## Generate coverage report + per-package gate (reporting-only in v0.4.2)
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./... 2>/dev/null || true
	$(GO) tool cover -html=coverage.out -o coverage.html
	$(GO) run ./cmd/coverage-gate -threshold 60 -allowlist .coverage-allowlist coverage.out
	@echo "→ coverage.html ready"

lint: ## go vet + staticcheck (pinned via go.mod tool directive — no install needed)
	$(GO) vet ./...
	$(GO) tool staticcheck ./...

clean: ## Remove built binaries + coverage artifacts + release dist/
	rm -rf bin/ dist/ coverage.out coverage.html

tidy: ## go mod tidy
	$(GO) mod tidy

mirror-install: ## Copy install.sh to docs/install/install.sh for GitHub Pages
	@mkdir -p docs/install
	@cp install.sh docs/install/install.sh
	@echo "→ docs/install/install.sh updated (mirror of repo-root install.sh)"

release-dry: ## Local dry-run of the release pipeline (no publish)
	$(GO) tool goreleaser release --snapshot --skip=publish --clean

release: ## Real release (intended for CI / git tag triggers only)
	@git describe --exact-match --tags HEAD >/dev/null 2>&1 || \
		{ echo "error: not on a vX.Y.Z tag; tag the commit first (git tag vX.Y.Z)"; exit 1; }
	$(GO) tool goreleaser release --clean
