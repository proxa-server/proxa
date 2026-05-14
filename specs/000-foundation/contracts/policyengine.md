# Contract: `internal/auth.PolicyEngine`

Authorization decisions: given a `Subject` and a target resource, return allow/deny. Lives in the same package as `Authenticator` (`internal/auth/`) because they're consumed together by middleware.

## Go signature (v0.0)

```go
package auth

import (
    "context"
    "errors"

    "github.com/proxa-server/proxa/pkg/types"
)

// PolicyEngine answers "may this subject perform this verb on this resource?"
type PolicyEngine interface {
    // Authorize returns nil if allowed, ErrForbidden if denied.
    Authorize(ctx context.Context, req AuthzRequest) error

    // PoliciesFor returns the bound policies for a subject (admin/dashboard view).
    PoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error)
}

type AuthzRequest struct {
    Subject  *types.Subject
    Verb     Verb              // get | list | create | update | delete | exec | logs
    Resource ResourceRef
}

type ResourceRef struct {
    Kind    ResourceKind // project | service | job | secret | node | policy | subject
    Project string       // "" means cluster-scope (only Kind=node|subject|policy with project="*")
    Name    string       // "" for kind-level operations (e.g., list)
}

type Verb string
const (
    VerbGet    Verb = "get"
    VerbList   Verb = "list"
    VerbCreate Verb = "create"
    VerbUpdate Verb = "update"
    VerbDelete Verb = "delete"
    VerbExec   Verb = "exec"
    VerbLogs   Verb = "logs"
)

type ResourceKind string
const (
    KindProject ResourceKind = "project"
    KindService ResourceKind = "service"
    KindJob     ResourceKind = "job"
    KindSecret  ResourceKind = "secret"
    KindNode    ResourceKind = "node"
    KindPolicy  ResourceKind = "policy"
    KindSubject ResourceKind = "subject"
)

var (
    ErrForbidden      = errors.New("auth: forbidden")
    ErrNotImplemented = errors.New("auth: policy engine not implemented")
)
```

## Behavioral contract

1. **Role semantics** (resolved against bound `types.Policy` rows):
   - `admin`: every verb on every resource across all projects.
   - `editor`: get/list/create/update/delete on services/jobs/secrets in scoped project; get/list on policies/subjects in scoped project.
   - `viewer`: get/list only, in scoped project.
   - `agent`: get on services/jobs in any project (read for reconciliation); update on node heartbeats for its own node ID; nothing else.

2. **Project scoping is mandatory** (constitution §III). A request with `Project=""` for a project-scoped kind is denied.

3. **Wildcards**: a policy with `Project="*"` matches every project but is only honored for `Role=admin`. Other roles' wildcard policies are rejected at `StateStore.PutPolicy` time.

4. **Authorize is read-only** against the store: it MAY cache `PoliciesFor` lookups in-memory with a short TTL; cache invalidation on policy mutation is the impl's responsibility.

5. **Audit hook reserved**: `Authorize` will gain a `(decision, reason)` audit emission via `slog` in a later feature. The interface does not change.

## Foundation deliverable

`internal/auth/policy.go` declares the interface + verb/kind enums + sentinel errors. No concrete policy engine in this feature.
