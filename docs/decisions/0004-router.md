# 4. Keep chi Router Until v1.0

## Status

Accepted (2026-05-21)

## Context

Proxa's HTTP server (API + UI) uses [`github.com/go-chi/chi/v5`](https://github.com/go-chi/chi) for routing. chi is a small (~3000 LOC) MIT-licensed router built on top of `net/http`. It provides:

- Method-prefixed routes (`r.Get(...)`, `r.Post(...)`).
- URL parameter extraction (`chi.URLParam(r, "name")`).
- Sub-router mounting (`r.Route(...)` for hierarchical paths).
- Per-route middleware composition (`r.With(...)`).
- A `Walk` API for route introspection.

Go 1.22 substantially upgraded the stdlib `net/http.ServeMux`:

- Method-prefixed patterns (`mux.HandleFunc("GET /foo/{id}", h)`).
- Path parameters with `r.PathValue("id")`.
- Precedence rules so more-specific patterns win.

This raises a recurring question: should Proxa drop chi and use the stdlib router directly, removing one dependency from the single binary?

## Decision

**Keep chi as the HTTP router until at least v1.0.** The migration to stdlib `net/http.ServeMux` is deferred indefinitely; revisit when one of the triggers below fires.

## Rationale

- **No daily-loop pain**: chi works correctly, performs well, and is invisible to operators. Migration would be ~300 LOC of mechanical replacement with no user-visible benefit.
- **Feature parity gap**: Go 1.22's `ServeMux` matches chi's *routing* features but not its *composition* features. chi's per-route middleware (`r.With(...)`) and sub-router mounting (`r.Route(...)`) have no direct stdlib equivalent — they would have to be reimplemented as ad-hoc wrappers around `ServeMux.Handle`. The migration is "replace one library with a thinner library plus 100 LOC of glue", not "replace one library with the stdlib".
- **Dependency cost is low**: chi is MIT-licensed, has zero transitive dependencies, ships ~3000 LOC of well-reviewed code, and is on a stable 1.x release line.
- **Foundation Train priorities are higher**: v0.4.x focuses on modernization, test foundation, and architectural primitives. v0.5+ introduces killer features (Confidence Mode, Post-Mortem Mode). A router migration is busywork that competes with those for the same scarce attention budget.
- **Constitution §V (Single Binary, Zero Deps) is honored**: chi is one well-isolated dependency. The constitution wants us to avoid *unnecessary* deps, not all deps; chi clears that bar.

## When to revisit

Trigger the chi → stdlib migration if any of:

1. **chi sees a maintenance gap** (no releases > 12 months, or a security advisory unpatched > 30 days). The migration becomes self-defense.
2. **A v1.x release deliberately rebuilds the routing/middleware stack** (for HTTP/3, structured tracing, OpenTelemetry, etc.) and the rebuild touches enough of the surface that the chi-isolation argument disappears.
3. **`net/http.ServeMux` gains first-class middleware composition** in a future Go release (rumored but not committed for 1.27+). At that point the "100 LOC of glue" argument flips.
4. **A team contributor proposes the migration with a working branch**. The first 200 LOC of conversion + green test suite is the strongest argument; until then, the speculative cost dominates.

Until one of those fires, the friction of migration > the benefit of removing one isolated dependency.

## Consequences

- chi remains a transitive dependency in `go.mod` until v1.0+. The license audit log (`docs/licenses.md`) confirms it as Apache-2.0-compatible (MIT).
- New API endpoints continue to use chi's routing primitives. Contributors do not need to learn the stdlib `ServeMux` pattern syntax for Proxa work.
- This ADR is the canonical answer when the question recurs (it has come up at least once in 005-modern-go's research phase). Future questioners can be redirected here.

## Related

- [ADR-0005 — Deferred modernizers](./0005-deferred-modernizers.md) — chi migration is one of the deferred items.
- [specs/005-modern-go/research.md](../../specs/005-modern-go/research.md) — the original research on Go 1.22-1.26 features that surfaced this question.
