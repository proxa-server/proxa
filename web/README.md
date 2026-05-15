# `web/`

Source-only landing for dashboard development assets. The actual
embedded files live under [`internal/web/`](../internal/web/) so
Go's `embed` directive can find them inside its package directory.

| Path | Purpose |
|---|---|
| [`web/_src/app.css`](_src/app.css) | Tailwind / hand-CSS source. Hand-edited; output committed to `internal/web/static/app.css`. |
| [`internal/web/static/`](../internal/web/static/) | Vendored frontend assets embedded into the binary. See its [README](../internal/web/static/README.md) for HTMX/Alpine pinned versions and bump procedure. |
| [`internal/web/templates/`](../internal/web/templates/) | `html/template` files (`index.html`, `services_table.html`) parsed at startup by `internal/web/web.go`. |

## Why two directories?

Go's `go:embed` directive can only embed files at-or-below the
embedding package's directory (`internal/web/web.go`). To support
that without forcing designers to edit files inside `internal/`
(which feels backend-y), the source-of-truth Tailwind input lives
at `web/_src/app.css` and the build/regenerate step copies the
output into `internal/web/static/`.

For v0.0 the regenerate step is `cp web/_src/app.css internal/web/static/app.css`
since we use hand-written CSS. When the full dashboard arrives in
Feature 004 with the proper Tailwind v4 toolchain:

```sh
tailwindcss -i web/_src/app.css -o internal/web/static/app.css --minify
```

The standalone `tailwindcss` CLI binary is used (single static binary
downloaded once); no Node.js at build or runtime, honoring §V.

## Reference mockup

A directional HTML mockup lives at
[`specs/_reference/dashboard-mockup.html`](../specs/_reference/dashboard-mockup.html).

The mockup captures the intended information architecture (sidebar
sections, project selector with role chips, node resource bars, log
viewer, TOML preview) and design tokens (sage/teal palette, Geist
typography). The slim v0.0 dashboard implements only the services
table; the rest lands with Feature 004.
