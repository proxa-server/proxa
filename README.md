# Proxa

> Starts like Kamal. Scales like Nomad. Licensed Apache 2.0.

Proxa is an open-source, self-hosted container orchestrator. A single Go
binary that embeds its control plane, HTTP/TCP ingress, admin dashboard,
scheduler, and CLI. Designed to be usable on a single node from day one
and to scale to multi-node clusters with mesh networking in v1.0.

## Status

**Pre-v0.0** — this repository is being scaffolded under Spec-Driven
Development. The current feature is
[`specs/000-foundation`](specs/000-foundation/spec.md), which lays out the
monorepo skeleton and the seven core interfaces every later feature
implements against.

## Repo layout

| Path | Purpose |
|---|---|
| `cmd/proxa/` | Control-plane binary entrypoint. |
| `cmd/proxa-agent/` | Per-node agent binary entrypoint. |
| `internal/runtime/` | `Runtime` interface — Docker (and later containerd) implements this. |
| `internal/store/` | `StateStore` interface — SQLite v0.x, etcd v1.0. |
| `internal/secrets/` | `SecretsStore` interface — age-backed in v0. |
| `internal/ingress/` | `IngressController` interface — L7 (CertMagic) + L4 (stdlib `net`). |
| `internal/auth/` | `Authenticator` and `PolicyEngine` interfaces. |
| `internal/security/` | `SecurityProfile` struct + `Default`/`Apply`/`Validate` (constitution §II). |
| `internal/version/` | Build-time version vars (set via `-ldflags`). |
| `pkg/types/` | Shared public types: `TaskDef`, `Service`, `Job`, `Node`, `Project`, `Policy`, `Subject`, `SecurityProfile`. |
| `proto/` | Reserved for gRPC (control-plane↔agent). |
| `web/` | Reserved for the embedded HTMX dashboard. |
| `specs/` | Spec-Driven Development feature folders. |
| `specs/_reference/` | Cross-feature reference artifacts (e.g., dashboard mockup). |
| `.specify/` | Spec Kit toolchain (constitution, scripts, templates). |

## Quickstart

See [`specs/000-foundation/quickstart.md`](specs/000-foundation/quickstart.md)
for the build/test/lint flow once the foundation feature lands.

## Workflow

This project uses [Spec Kit](https://github.com/github/spec-kit) for
spec-driven development.

```
/speckit.specify <feature>     → author or update a feature spec
/speckit.plan                  → produce plan.md from spec.md
/speckit.tasks                 → produce dependency-ordered tasks.md
/speckit.analyze               → cross-check spec/plan/tasks consistency
/speckit.implement             → execute tasks.md (one commit per task)
```

## Constitution

Non-negotiable principles for every contribution live in
[`.specify/memory/constitution.md`](.specify/memory/constitution.md):
interfaces first, security by default, project scoping from v0.0, Go
idioms, single binary with zero external deps, cluster-ready design,
declarative reconciliation, zero-downtime deploys, Apache 2.0 license,
honest scope, structured commits.

## License

Apache 2.0.
