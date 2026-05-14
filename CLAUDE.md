# Proxa — Project Instructions

Proxa is a self-hosted container orchestrator built as a single Go binary. Apache 2.0 licensed. Development uses Spec-Driven Development via Spec Kit.

## Source of truth (read these before doing work)

1. **Constitution** — `.specify/memory/constitution.md`. Non-negotiable principles. Every plan, task, and implementation must comply.
2. **Feature specs** — `specs/<NNN-feature-name>/spec.md`. Each feature has its own folder. Current features: `000-foundation`, `001-core-loop`. (The speckit toolchain hardcodes this location; do not move specs back under `.specify/specs/`.)
3. **Technical spec** — the original `proxa-spec.docx` (not in repo) defines the 18-section design. The feature specs are derived from it.

## Tech stack (do not deviate without updating the constitution)

- Go 1.26.x, `CGO_ENABLED=0`, single static binary
- chi HTTP router, cobra+viper CLI
- modernc.org/sqlite (single-node), etcd embedded (multi-node v1.0)
- Docker API via `docker/docker/client` (behind `Runtime` interface)
- CertMagic for L7 TLS, Go stdlib `net` for L4 TCP/UDP
- age for secret encryption
- HTMX + Alpine.js + Tailwind embedded via `go:embed` (no Node.js)
- slog (stdlib) for structured logs as JSON to stderr
- Tailscale tsnet + Headscale for mesh (v1.0+)

## Workflow

Use the speckit skills (`.claude/skills/`):

- `/speckit.specify` — author or update a feature spec
- `/speckit.plan` — produce `plan.md` from a `spec.md`
- `/speckit.tasks` — break a plan into ordered, dependency-aware tasks
- `/speckit.implement` — execute the tasks
- `/speckit.analyze` — cross-check spec/plan/tasks consistency
- `/speckit.clarify` — surface underspecified areas in a spec

Commit format (enforced by constitution §XI):

```
<type>(<scope>): <description>
```

Types: `feat`, `fix`, `test`, `docs`, `refactor`, `chore`. Scope matches the `internal/<package>` name.

## What this repo USED to be

Until May 2026 this repo held a Python ASGI/WSGI autoscaler with Envoy integration. That code is preserved on the `legacy-python` branch. **Do not import, copy, or reference it.** The current project shares only the name and the GitHub repo URL.
