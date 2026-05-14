// Package dbpolicy implements [auth.PolicyEngine] backed by the
// StateStore's policies table. Authorization decisions consult the
// list of policies bound to the subject and match against the
// (Verb, Kind, Project) tuple of the request.
//
// See specs/000-foundation/contracts/policyengine.md for role semantics.
package dbpolicy

import (
	"context"
	"fmt"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/pkg/types"
)

// PolicyLister is the minimum store surface this engine needs.
type PolicyLister interface {
	ListPoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error)
}

// Engine is the StateStore-backed PolicyEngine.
type Engine struct {
	store PolicyLister
}

// New returns an Engine backed by the given store.
func New(store PolicyLister) *Engine {
	return &Engine{store: store}
}

// Authorize answers "may this subject perform this verb on this resource?"
// Returns nil when allowed, [auth.ErrForbidden] when denied.
func (e *Engine) Authorize(ctx context.Context, req auth.AuthzRequest) error {
	if req.Subject == nil {
		return auth.ErrForbidden
	}
	policies, err := e.store.ListPoliciesFor(ctx, req.Subject.ID)
	if err != nil {
		return fmt.Errorf("auth/dbpolicy: list policies: %w", err)
	}

	// Reject empty Project for project-scoped kinds.
	if isProjectScoped(req.Resource.Kind) && req.Resource.Project == "" {
		return auth.ErrForbidden
	}

	for _, p := range policies {
		if matches(p, req) {
			return nil
		}
	}
	return auth.ErrForbidden
}

// PoliciesFor returns the policies bound to a subject. Surface for
// admin/dashboard views.
func (e *Engine) PoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error) {
	return e.store.ListPoliciesFor(ctx, subjectID)
}

// matches returns true if policy p grants the request.
func matches(p types.Policy, req auth.AuthzRequest) bool {
	switch p.Role {
	case types.RoleAdmin:
		// Admin can do anything in any project (or all projects).
		return p.Project == "*" || p.Project == req.Resource.Project ||
			!isProjectScoped(req.Resource.Kind)

	case types.RoleEditor:
		if p.Project != req.Resource.Project {
			return false
		}
		switch req.Resource.Kind {
		case auth.KindService, auth.KindJob, auth.KindSecret:
			return true // CRUD allowed
		case auth.KindProject, auth.KindPolicy, auth.KindSubject:
			// Editors can read but not write project/policy/subject
			return req.Verb == auth.VerbGet || req.Verb == auth.VerbList
		}
		return false

	case types.RoleViewer:
		if p.Project != req.Resource.Project {
			return false
		}
		return req.Verb == auth.VerbGet || req.Verb == auth.VerbList

	case types.RoleAgent:
		switch req.Resource.Kind {
		case auth.KindService, auth.KindJob:
			return req.Verb == auth.VerbGet || req.Verb == auth.VerbList
		case auth.KindNode:
			// Agents update their own heartbeats; v0.0 does not enforce
			// "own node" — a future hardening pass adds that check.
			return req.Verb == auth.VerbUpdate || req.Verb == auth.VerbGet
		}
		return false
	}
	return false
}

// isProjectScoped reports whether the resource kind requires a
// non-empty Project on the request.
func isProjectScoped(k auth.ResourceKind) bool {
	switch k {
	case auth.KindService, auth.KindJob, auth.KindSecret:
		return true
	}
	return false
}
