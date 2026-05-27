// Package harness holds the shared helpers used by Proxa's end-to-end
// test suite. It consolidates infrastructure that previously lived as
// package-private helpers scattered across tests/e2e/*.go files.
//
// Files (one concern per file):
//
//   - proxa.go     — runProxa, ProxaBinary, FindRepoRoot
//   - server.go    — StartServer (background subprocess + cleanup)
//   - socket.go    — SocketPath, GetViaSocket, SSERequest (Unix-socket HTTP clients)
//   - docker.go    — WaitForCount, WaitForServiceStatus, SkipIfHTTPProbeUnreachable
//   - token.go     — ReadToken
//   - ports.go     — PickTwoFreeTCPPorts
//   - snippet.go   — Snippet (string truncation for failure output)
//   - copyrepo.go  — CopyRepoForTest (for fresh-clone smoke tests)
//   - images.go    — pinned container image digests (for digest-pinned tests)
//   - attrs.go     — SCAttrs (T.Attr-based test tagging for spec/SC traceability)
//
// Usage: every tests/e2e/*_test.go file imports this package and calls
// helpers via harness.X(...). Build-tagged `e2e` because the package
// only makes sense in the e2e context (uses os/exec, docker CLI, etc.).
package harness
