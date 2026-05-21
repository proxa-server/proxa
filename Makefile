# Proxa — make targets
#
# CGO is disabled everywhere (constitution §IV). Build-time identity is
# injected via -ldflags into internal/version. Staticcheck is run via
# `go run` so contributors do not need a global install.

GO              ?= go
STATICCHECK     := honnef.co/go/tools/cmd/staticcheck@v0.7.0   # legacy: see `go tool` directive in go.mod (v0.4.1+)
VERSION         := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT          := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS         := -X github.com/proxa-server/proxa/internal/version.Version=$(VERSION) \
                   -X github.com/proxa-server/proxa/internal/version.Commit=$(COMMIT) \
                   -X github.com/proxa-server/proxa/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: build test lint clean tidy help

help: ## List targets
	@awk 'BEGIN{FS=":.*##"; printf "Targets:\n"} /^[a-zA-Z_-]+:.*##/ {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build proxa and proxa-agent into bin/
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/proxa ./cmd/proxa
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/proxa-agent ./cmd/proxa-agent

test: ## Run all tests
	$(GO) test -race -count=1 ./...

test-integration: ## Run dockerd-tagged integration tests (requires Docker daemon)
	$(GO) test -race -count=1 -tags dockerd ./...

test-e2e: build ## Run e2e tests against the built binary (requires Docker daemon)
	$(GO) test -count=1 -tags e2e ./tests/e2e/...

lint: ## go vet + staticcheck (pinned via go.mod tool directive — no install needed)
	$(GO) vet ./...
	$(GO) tool staticcheck ./...

clean: ## Remove built binaries
	rm -rf bin/

tidy: ## go mod tidy
	$(GO) mod tidy
