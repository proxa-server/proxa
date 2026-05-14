# Proxa

> Starts like Kamal. Scales like Nomad. Licensed Apache 2.0.

Proxa is an open-source, self-hosted container orchestrator. A single Go binary that embeds its control plane, HTTP/TCP ingress, admin dashboard, scheduler, and CLI. Designed to be usable on a single node from day one, and to scale to multi-node clusters with WireGuard mesh networking in later releases.

## Status

**Pre-v0.0** — this repository is being scaffolded under Spec-Driven Development. See:

- [`.specify/memory/constitution.md`](.specify/memory/constitution.md) — non-negotiable principles
- [`.specify/specs/000-foundation/spec.md`](.specify/specs/000-foundation/spec.md) — monorepo scaffolding
- [`.specify/specs/001-core-loop/spec.md`](.specify/specs/001-core-loop/spec.md) — reconciliation engine

The full technical specification (`proxa-spec.docx`) defines architecture, clustering model, security, projects, auth/RBAC, storage, CLI, dashboard, core Go interfaces, and release phases.

## Workflow

This project uses [Spec Kit](https://github.com/github/spec-kit) for spec-driven development.

```
/speckit.plan <feature>      → produces plan.md from spec.md
/speckit.tasks               → produces tasks.md from plan.md
/speckit.implement           → executes tasks.md
```

## License

Apache 2.0.
