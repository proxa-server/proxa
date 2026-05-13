# Quickstart — Foundation

After this feature lands, a fresh clone gives you a buildable, lintable, testable Go monorepo with the seven core interfaces declared. Nothing user-facing works yet — that's the next feature.

## Prerequisites

- Go 1.26.x (`go version` → `go1.26.x`)
- `make` (BSD or GNU)
- `git`

Optional for local linting:
- `staticcheck` (`go install honnef.co/go/tools/cmd/staticcheck@latest`)

## First-touch flow

```sh
git clone https://github.com/proxa-server/proxa
cd proxa
git checkout 000-foundation   # or main once merged

make build      # CGO_ENABLED=0 go build ./...
make test       # go test ./...
make lint       # go vet + staticcheck

./bin/proxa version
# proxa version dev (commit unknown, built unknown)
```

## What you should see

| Path | Purpose |
|---|---|
| `cmd/proxa/main.go` | Control-plane binary; today prints version only. |
| `cmd/proxa-agent/main.go` | Per-node agent binary; today prints version only. |
| `internal/runtime/runtime.go` | `Runtime` interface — Docker (and later containerd) implements this. |
| `internal/store/store.go` | `StateStore` interface — SQLite v0, etcd v1.0. |
| `internal/secrets/secrets.go` | `SecretsStore` interface — age-backed in v0. |
| `internal/ingress/ingress.go` | `IngressController` interface — L7 + L4 ingress. |
| `internal/auth/auth.go` | `Authenticator` interface + `Chain` helper. |
| `internal/auth/policy.go` | `PolicyEngine` interface + RBAC verb/kind enums. |
| `internal/security/profile.go` | `SecurityProfile` struct + `Default/Apply/Validate` helpers (only real logic in this feature). |
| `pkg/types/` | `TaskDef`, `Service`, `Job`, `Node`, `Project`, `Policy`, `Subject`. |
| `proto/` | Reserved for gRPC; README only. |
| `web/` | Reserved for dashboard; README only. |
| `.github/workflows/ci.yml` | `vet` + `staticcheck` + `test` + `build`. |
| `.goreleaser.yml` | Multi-arch release scaffold; not yet wired to a tag. |

## Implementing against the interfaces (for AI agents)

When a later feature asks you to implement, say, the Docker `Runtime`:

1. Read `specs/000-foundation/contracts/runtime.md` for the behavioral contract.
2. Create `internal/runtime/docker.go` implementing the interface.
3. Add `github.com/docker/docker/client` to `go.mod` (it was *not* added in Foundation per FR-009).
4. Write table-driven tests in `internal/runtime/docker_test.go`. Use a fake/stub for unit tests; reserve a `docker_integration_test.go` build-tagged with `//go:build integration` for tests that need a live daemon.
5. Commit per constitution §XI: `feat(runtime): implement Docker container creation via Runtime interface`.

## Success criteria (verify before declaring this feature done)

- [ ] `go build ./...` exits 0 on a clean clone with `CGO_ENABLED=0`.
- [ ] `go vet ./...` reports zero issues.
- [ ] `staticcheck ./...` reports zero issues.
- [ ] `go test ./...` passes (table-driven test for `security.Default/Apply/Validate` is the only meaningful test).
- [ ] `./bin/proxa version` prints a version string.
- [ ] `./bin/proxa-agent version` prints a version string.
- [ ] GitHub Actions CI is green on `000-foundation` branch.
- [ ] Every interface file has a doc comment on the type that links back to the contract spec (`see specs/000-foundation/contracts/<name>.md`).

## Out of scope (deferred to 001+)

- TOML parsing
- HTTP server / chi router wiring
- SQLite schema / migrations
- Docker client wiring
- Dashboard
- ACME / CertMagic
- Secrets encryption with age
- gRPC schemas in `proto/`
- Multi-arch release publishing (the goreleaser config exists but tags don't fire it yet)
