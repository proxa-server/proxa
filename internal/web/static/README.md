# `web/static/`

Frontend assets vendored for the slim dashboard. All embedded into the
Proxa binary via `go:embed` (constitution §V — no Node.js at build or
runtime).

## Pinned versions

| File | Version | Source |
|---|---|---|
| `htmx.min.js` | HTMX 2.0.4 | https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js |
| `alpine.min.js` | Alpine.js 3.13.10 | https://cdn.jsdelivr.net/npm/alpinejs@3.13.10/dist/cdn.min.js |
| `app.css` | hand-written | source: `web/_src/app.css` |

## How to bump

```sh
curl -sSL -o web/static/htmx.min.js   https://unpkg.com/htmx.org@<NEW_VERSION>/dist/htmx.min.js
curl -sSL -o web/static/alpine.min.js https://cdn.jsdelivr.net/npm/alpinejs@<NEW_VERSION>/dist/cdn.min.js
# Update the version pins in this file.
```

## Note on Tailwind

The slim dashboard in v0.0 uses hand-written CSS in `web/_src/app.css`
that mirrors Tailwind utility-class conventions but does not require
the Tailwind toolchain. When the full dashboard arrives in Feature 004,
this transitions to a real Tailwind v4 setup (`tailwindcss -i web/_src/app.css -o web/static/app.css --minify`)
with the `@theme` directive carrying the same sage/teal palette.

The constitution §V "no Node.js" rule is honored either way:
- v0.0: pure hand-CSS, no toolchain.
- v0.1+: Tailwind via the standalone `tailwindcss` CLI binary
  (single static binary downloadable from the Tailwind project), run
  manually as a one-shot when CSS changes — never invoked from CI.
