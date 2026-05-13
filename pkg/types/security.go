package types

import "github.com/proxa-server/proxa/internal/security"

// SecurityProfile is re-exported from internal/security so that public
// callers reach the canonical type through pkg/types alongside the other
// shared entities. The struct, [security.Default], [security.Apply], and
// [security.Validate] live in internal/security; this alias keeps them
// reachable via the public package.
//
// See specs/000-foundation/contracts/securityprofile.md.
type SecurityProfile = security.SecurityProfile
