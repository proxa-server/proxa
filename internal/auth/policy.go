package auth

import (
	"context"
	"errors"

	"github.com/proxa-server/proxa/pkg/types"
)

// PolicyEngine answers "may this subject perform this verb on this
// resource?" Authorize returns nil when allowed, [ErrForbidden] when
// denied.
//
// Behavioral rules (full contract in
// specs/000-foundation/contracts/policyengine.md):
//
//   - Project scoping is mandatory for project-scoped resource kinds.
//   - Wildcard policies (Project="*") are honored only for RoleAdmin;
//     other roles' wildcards are rejected at write time by the
//     [StateStore].
//   - Authorize is read-only against the store; impls MAY cache
//     PoliciesFor lookups with a short TTL.
type PolicyEngine interface {
	Authorize(ctx context.Context, req AuthzRequest) error
	PoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error)
}

// AuthzRequest is one authorization decision request.
type AuthzRequest struct {
	Subject  *types.Subject
	Verb     Verb
	Resource ResourceRef
}

// ResourceRef identifies one target resource. Project is empty only for
// cluster-scoped kinds (Node, Subject, Policy with project="*").
type ResourceRef struct {
	Kind    ResourceKind
	Project string
	Name    string // empty for kind-level operations (e.g., list)
}

// Verb is one of the canonical RBAC actions Proxa supports.
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

// ResourceKind enumerates Proxa's authorization-relevant resource types.
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

// ErrForbidden is returned by [PolicyEngine.Authorize] when the request
// is denied by policy.
var ErrForbidden = errors.New("auth: forbidden")

// noopPolicyEngine satisfies [PolicyEngine] with ErrNotImplemented for
// every method. Useful as a placeholder in unit tests.
type noopPolicyEngine struct{}

// Compile-time assertion that noopPolicyEngine satisfies PolicyEngine.
var _ PolicyEngine = noopPolicyEngine{}

func (noopPolicyEngine) Authorize(context.Context, AuthzRequest) error { return ErrNotImplemented }
func (noopPolicyEngine) PoliciesFor(context.Context, string) ([]types.Policy, error) {
	return nil, ErrNotImplemented
}
