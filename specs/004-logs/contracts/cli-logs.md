# Contract: `proxa logs <service>` CLI subcommand

## Synopsis

```text
proxa logs [flags] <service>

Flags:
  -f, --follow              stream new lines until Ctrl-C
  -n, --tail int            number of recent lines to fetch (-1 = all) (default -1)
      --since duration      only lines from the last DURATION (e.g., 5m, 1h30m)
      --replica int         replica index (default 0)
      --project string      project name (default "default")
      --timestamps          prefix each line with Docker's RFC3339 timestamp
                            (POLISH — not implemented in v0.4)
```

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success (one-shot completed OR follow exited cleanly on Ctrl-C / container exit) |
| 1 | generic error (auth, resolution, runtime) |
| 2 | flag validation error (bad `--since`, negative `--tail`, etc.) |
| 130 | exited via SIGINT (Ctrl-C) — standard convention |

## Output shape

```text
=== proxa-default-web-0 (replica 0) ===
2026-05-17T11:23:45Z first log line
2026-05-17T11:23:46Z second log line
...
```

The header (`=== container-name (replica N) ===`) prints once to stderr so it doesn't interfere with stdout-pipe consumers (e.g., `proxa logs web | grep ERROR`). The log lines go to stdout.

## Behavior matrix

| Invocation | Stream type | Exit |
|---|---|---|
| `proxa logs web` | one-shot, all lines | 0 |
| `proxa logs web --tail 50` | one-shot, last 50 lines | 0 |
| `proxa logs web -f` | follow until Ctrl-C / container exit | 0 (or 130 on Ctrl-C) |
| `proxa logs web --since 5m` | one-shot, last 5 minutes | 0 |
| `proxa logs web --replica 2` | one-shot, replica 2 only | 0 |
| `proxa logs nosuch` | n/a | 1 (`service-not-found`) |
| `proxa logs web --since broken` | n/a | 2 (`invalid --since value`) |
| `proxa logs web --replica 99` | n/a | 1 (`replica-not-found`) |
| `proxa logs web` (no proxa server) | n/a | 1 (`cannot reach proxa server`) |

## Signal handling

- `SIGINT` (Ctrl-C): the cobra cmd context cancels → HTTP request ctx cancels → server-side stream closes → response Body returns EOF → CLI prints nothing extra → exits 130.
- `SIGTERM`: same as SIGINT, exits 143.

The TCP connection MUST be closed before the process exits. Verified by SC-003 (no orphaned ESTABLISHED in lsof).

## Error messages — verbatim

```text
$ proxa logs nosuch
error: service "nosuch" not found in project "default"

$ proxa logs web --since broken
error: invalid --since value "broken": time: invalid duration "broken"

$ proxa logs web --replica 99
error: replica 99 not found (service has 3 replicas)

$ proxa logs web --tail -99
error: invalid --tail value -99: must be >= -1

$ proxa logs web   # server not running
error: cli: cannot reach proxa server at unix:///.../proxa.sock: dial unix ...
```

Stable error codes are NOT printed in the CLI output (they belong to the API consumer; the CLI's job is human-readable).

## Streaming guarantees

- First byte to stdout within 2s of command start (SC-001).
- In follow mode, new container lines reach stdout within 1s of being written (SC-002).
- Output is line-buffered; partial lines do not appear until terminated by `\n`.

## Examples

```bash
# Get the last 200 lines of the web service
proxa logs web --tail 200

# Follow the logs while running a test in another terminal
proxa logs web -f

# Recent lines only — last 10 minutes
proxa logs web --since 10m

# Specific replica
proxa logs api --replica 2 -f

# Pipe to grep — only the lines go through stdout
proxa logs web -f | grep ERROR
```
