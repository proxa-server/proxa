# Architectural Decision Records

This directory holds Proxa's architectural decision records (ADRs). Each ADR captures a single decision — its context, the chosen option, and the consequences — so future contributors can re-litigate from the same footing as the original author.

## Format

Lightweight Markdown in the [Michael Nygard style](https://github.com/joelparkerhenderson/architecture-decision-record/blob/main/locales/en/templates/decision-record-template-by-michael-nygard/index.md):

```markdown
# NNNN. Short title

## Status

Accepted | Superseded by NNNN | Deprecated

## Context

What is the issue we're seeing that motivates this decision?

## Decision

What is the change we're making?

## Consequences

What becomes easier? What becomes harder? What are the trade-offs?
```

## Numbering

Sequential 4-digit IDs. IDs 0001-0003 are reserved for retrofitting the foundation / health-checks / ingress features as ADRs if a future contributor decides those decisions deserve formal records. New ADRs start at 0004.

| ID | Title | Status |
|---|---|---|
| 0001 | _(reserved)_ foundation architecture | _(not yet retrofit)_ |
| 0002 | _(reserved)_ health checks design | _(not yet retrofit)_ |
| 0003 | _(reserved)_ ingress controller design | _(not yet retrofit)_ |
| 0004+ | _(new ADRs)_ | _(authored as work happens)_ |

## When to author an ADR

- A non-obvious decision was made between two reasonable options.
- A decision was deliberately deferred and the reasoning is worth preserving.
- A constraint was discovered that materially shapes future work.
- A constitution principle was interpreted in a way a reasonable contributor might re-question.

Routine implementation choices that follow obvious patterns do NOT need an ADR — those belong in the spec or commit message.
