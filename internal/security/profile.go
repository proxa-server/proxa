// Package security defines the canonical container security profile used
// across Proxa. The zero value of [SecurityProfile] MUST be the safest
// possible configuration (constitution §II). Helpers in this package
// ([Default], [Apply], [Validate]) make that contract enforceable at the
// API boundary.
//
// See specs/000-foundation/contracts/securityprofile.md for the full
// behavioral contract.
package security

import "errors"

// SecurityProfile is the set of per-container security knobs that a Runtime
// implementation passes to the container backend. Zero value yields a fully
// locked-down container after [Apply] has been called on it.
type SecurityProfile struct {
	// User is the UID:GID the container runs as. An empty string means
	// "non-root, runtime picks". Setting "root" or "0:0" is a hard error
	// during [Validate] unless AllowRoot is explicitly true.
	User string `toml:"user" json:"user,omitempty"`

	// AllowRoot is the escape hatch that permits User="root" and
	// NoNewPrivileges=false. Required to be explicit at the TaskDef level
	// so that escaping the secure defaults is visible at PR review time.
	AllowRoot bool `toml:"allowRoot" json:"allowRoot,omitempty"`

	// CapAdd is the list of Linux capabilities to grant. Empty by default.
	CapAdd []string `toml:"capAdd" json:"capAdd,omitempty"`

	// CapDrop is the list of Linux capabilities to drop. Default ["ALL"].
	CapDrop []string `toml:"capDrop" json:"capDrop,omitempty"`

	// NoNewPrivileges prevents the container from gaining privileges via
	// setuid. Default true; cannot be disabled unless AllowRoot is also true.
	// Pointer so we can distinguish "unset" from "explicitly false".
	NoNewPrivileges *bool `toml:"noNewPrivileges" json:"noNewPrivileges,omitempty"`

	// ReadOnlyRootFS mounts / as read-only. Default false; flagged as a
	// hardening recommendation in the dashboard.
	ReadOnlyRootFS bool `toml:"readOnlyRootFs" json:"readOnlyRootFs,omitempty"`

	// SeccompProfile is the seccomp profile name. Empty = runtime default.
	SeccompProfile string `toml:"seccompProfile" json:"seccompProfile,omitempty"`

	// AppArmorProfile is the AppArmor profile name. Empty = runtime default.
	AppArmorProfile string `toml:"appArmorProfile" json:"appArmorProfile,omitempty"`
}

// Default returns the constitution-compliant default security profile:
// non-root user, all capabilities dropped, no new privileges.
func Default() SecurityProfile {
	t := true
	return SecurityProfile{
		User:            "",
		AllowRoot:       false,
		CapAdd:          nil,
		CapDrop:         []string{"ALL"},
		NoNewPrivileges: &t,
		ReadOnlyRootFS:  false,
	}
}

// Apply fills zero-valued fields with the values from [Default] without
// overriding caller choices. Runtime implementations MUST call Apply before
// handing the spec to the container backend, ensuring constitution §II is
// honored even when callers under-specify.
func Apply(p SecurityProfile) SecurityProfile {
	d := Default()
	if p.CapDrop == nil {
		p.CapDrop = d.CapDrop
	}
	if p.NoNewPrivileges == nil {
		p.NoNewPrivileges = d.NoNewPrivileges
	}
	return p
}

// Validate enforces constitution §II at the API boundary. It rejects
// configurations that escape the secure defaults without an explicit
// AllowRoot opt-in. Callers should run Validate during TaskDef parsing
// so users see a fixable error message rather than a runtime surprise.
func Validate(p SecurityProfile) error {
	if (p.User == "root" || p.User == "0" || p.User == "0:0") && !p.AllowRoot {
		return errors.New("security: user 'root' requires allowRoot=true")
	}
	if p.NoNewPrivileges != nil && !*p.NoNewPrivileges && !p.AllowRoot {
		return errors.New("security: noNewPrivileges=false requires allowRoot=true")
	}
	return nil
}
