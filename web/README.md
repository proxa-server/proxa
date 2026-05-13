# `web/`

Reserved directory for the Proxa dashboard. Empty in feature `000-foundation`.

When populated, this directory will hold the HTMX + Alpine.js + Tailwind
assets for the embedded admin dashboard, surfaced into the binary via
`go:embed` (constitution §V — no Node.js at build or runtime).

## Reference mockup

A directional HTML mockup lives at
[`specs/_reference/dashboard-mockup.html`](../specs/_reference/dashboard-mockup.html).

The mockup captures the intended information architecture (sidebar
sections, project selector with role chips, node resource bars, log
viewer, TOML preview) and design tokens (sage/teal palette, Geist
typography). It is **not** the final implementation — it uses vanilla
CSS and external CDNs, both of which deviate from the constitutional
tech stack. The dashboard feature's `/speckit.plan` will reconcile.
