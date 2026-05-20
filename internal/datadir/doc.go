// Package datadir provides a path-sandboxed wrapper around the Proxa
// data directory and a helper for snapshotting its contents.
//
// All file operations on the Proxa data directory MUST flow through
// the Root type defined here. Root wraps Go 1.24's *os.Root, so any
// name that resolves outside the anchor directory — through a parent
// reference, an absolute path, or a symlink target — is refused at
// the OS layer. The path-traversal guarantee comes from the standard
// library; this package adds the small ergonomic surface Proxa uses.
//
// Snapshot copies the contents of a Root to a caller-supplied
// destination path, atomically from the caller's perspective (write
// to a sibling temp directory, then rename). The helper is library-
// only in v0.4.1 — it exists so that v0.4.3 architectural work and
// v0.5.0 rollback can both consume the same contract without
// refactoring the data-directory access layer.
package datadir
