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
//
// The production implementation lives in
// [github.com/proxa-server/proxa/internal/auth/dbpolicy]; the noop
// stub from feature 000 was removed once dbpolicy.New() became the
// real impl. Tests that need a fake should hand-roll one.
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
