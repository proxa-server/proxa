# Proxa Server — Constitution

> Governing principles for all Proxa development. Every spec, plan, task, and implementation must comply.

## I. Architecture First (NON-NEGOTIABLE)

Every component is designed behind a Go interface before implementation begins. The interface defines the contract; the implementation is swappable. This is how single-node SQLite becomes multi-node etcd without rewriting handlers.

Interfaces that exist from day one: `Runtime`, `StateStore`, `SecretsStore`, `IngressController`, `Authenticator`, `PolicyEngine`, `SecurityProfile`.

## II. Security by Default (NON-NEGOTIABLE)

No container runs as root. No API endpoint listens without authentication. No secret appears in logs, API responses, or the dashboard. Every container gets `CapDrop: ALL` and `NoNewPrivileges: true` unless the task definition explicitly overrides.

Security is not a feature gate or a post-hoc addition. It is a property of every line of code.

## III. Project Scoping from v0.0 (NON-NEGOTIABLE)

Every resource (service, job, secret, config map, ingress route, deployment, policy) has a `project` field in the state store. There is no global flat namespace. A `default` project exists for convenience, but the data model always scopes by project.

This prevents the Kubernetes retrofit problem where namespaces were bolted on years later.

## IV. Go Idioms

- Standard library first. Only add a dependency when the stdlib genuinely cannot do the job.
- `context.Context` on every function that does I/O.
- Errors are values, not panics. Wrap with `fmt.Errorf("component: %w", err)`.
- Structured logging via `slog` (stdlib Go 1.21+), JSON to stderr.
- Table-driven tests. No test frameworks beyond `testing` and `testify/assert` if needed.
- No CGO. The binary must compile with `CGO_ENABLED=0`.

## V. Single Binary, Zero Dependencies

The `proxa` binary embeds everything: API server, scheduler, ingress, dashboard, DNS, CLI. The user installs one file and runs it. No Docker Compose, no sidecar processes, no build steps.

The dashboard is HTMX + Alpine.js + Tailwind embedded via `go:embed`. No Node.js required at build or runtime.

## VI. Cluster-Ready Design

Even in single-node v0.x, every architectural decision must not block multi-node clustering in v1.0. The scheduler is a component, not inline code. The state store is an interface. Container operations go through the Runtime interface. The auth model has an `agent` role. When clustering arrives, it plugs into existing seams.

## VII. Declarative, Not Imperative

Users describe desired state in TOML files. Proxa reconciles actual state to match. The reconciliation loop is the heart of the system — it runs continuously, comparing what should exist with what does exist, and acting on the diff.

## VIII. Zero-Downtime by Default

Deployments use start-first strategy for stateless services (new containers pass health checks before old ones stop) and stop-first for stateful services (to prevent data corruption). Rollback is instant because the previous version's image and config are retained in deployment history.

## IX. Permissive License (NON-NEGOTIABLE)

Apache 2.0. Every dependency must be compatible (Apache, MIT, BSD, MPL 2.0). No BSL, no SSPL, no AGPL, no proprietary dependencies. This is verified before any new `go get` and documented in the spec's dependency table.

## X. Honest Scope

Proxa is a learning project and portfolio piece with well-defined release phases. Features are implemented incrementally, not promised speculatively. If something is not in the current release scope, it is documented in "Explicit Non-Goals" and not half-implemented.

## XI. Commit Strategy

Every completed task requires its own commit with a structured message:

```
<type>(<scope>): <description>

feat(runtime): implement Docker container creation via Runtime interface
fix(scheduler): handle nil pointer when no nodes are registered
test(auth): add table-driven tests for TokenAuthenticator
docs(spec): add clustering model section to technical spec
```

Types: `feat`, `fix`, `test`, `docs`, `refactor`, `chore`. Scope matches the `internal/` package name.
