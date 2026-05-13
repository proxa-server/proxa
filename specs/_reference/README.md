# `specs/_reference/`

Cross-feature reference artifacts. Not a feature — the leading underscore keeps this directory out of the speckit numerical scan (`000-`, `001-`, ...).

Files here inform multiple future features. Treat them as **directional**, not authoritative: every artifact here predates its corresponding feature spec and will be reconciled with the constitution + spec during that feature's `/speckit.plan` pass.

## Index

| File | Informs | Notes |
|---|---|---|
| `dashboard-mockup.html` | Future dashboard feature | Standalone HTML mockup of the Proxa dashboard. Captures IA + design tokens (sage/teal palette, Geist typography, sidebar sections, project selector with role chips, node resource bars, log viewer, TOML preview). Uses vanilla CSS + Alpine + CDN assets — **NOT** the final tech stack. See memory `project-dashboard-reference` for the deviation list. |
