package types

import "time"

// Policy binds a [Subject] to a [Role] within a project scope. The
// [PolicyEngine] in internal/auth resolves Authorize requests against
// the set of policies for a given subject.
type Policy struct {
	ID        string    `json:"id"`
	SubjectID string    `json:"subjectId"`
	Role      Role      `json:"role"`
	Project   string    `json:"project"` // "*" = all projects (admin only)
	CreatedAt time.Time `json:"createdAt"`
}

// Role enumerates Proxa's RBAC roles. Wildcards on Project ("*") are
// only honored for RoleAdmin; the StateStore rejects wildcard policies
// for any other role at write time.
type Role string

const (
	RoleAdmin  Role = "admin"  // all projects, all verbs
	RoleEditor Role = "editor" // CRUD on services/jobs/secrets in scoped project
	RoleViewer Role = "viewer" // read-only in scoped project
	RoleAgent  Role = "agent"  // cluster nodes calling back to the control plane
)
