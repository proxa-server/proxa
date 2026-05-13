# `proto/`

Reserved directory for the gRPC schema that the control plane and per-node
agents speak. Empty in feature `000-foundation`; the schema lands when the
control plane↔agent loop is implemented (target: feature `002-clustering`
or earlier if needed by `001-core-loop`).

When this directory is populated it will hold:

- `*.proto` — schema definitions (one file per service domain).
- `gen/` — generated Go bindings (`buf generate` or equivalent), checked in
  so contributors do not need a protoc toolchain to build Proxa.
