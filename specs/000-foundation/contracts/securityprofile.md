# Contract: `internal/security.SecurityProfile`

A **struct**, not an interface — it's the canonical container security knobs that pass from `TaskDef` to `Runtime.CreateContainer`. Lives in `internal/security/` so the package can also host helper functions (`Default()`, `Validate()`) without polluting `pkg/types/`.

> Note: `pkg/types/taskdef.go` also embeds a `SecurityProfile` field. To avoid an import cycle (`pkg/types` cannot import `internal/`), the struct is **defined in `internal/security/profile.go` and re-exported via a type alias in `pkg/types/security.go`**. This keeps the canonical definition in `internal/` while letting public consumers reference it through `pkg/types`. The alternative — defining it in `pkg/types` and having helpers in `internal/security`  reference it — also works; we pick the alias approach so non-stdlib helpers (validation, defaults) stay out of the public `pkg/types`.

## Go definition (v0.0)

```go
package security

// SecurityProfile is the set of per-container security knobs.
// Zero value MUST be the safest possible configuration (constitution §II).
type SecurityProfile struct {
    // User is the UID:GID the container runs as. "" means "non-root, runtime picks".
    // Setting to "root" or "0:0" is a hard error during validation unless
    // AllowRoot is explicitly true.
    User      string `toml:"user"      json:"user,omitempty"`
    AllowRoot bool   `toml:"allowRoot" json:"allowRoot,omitempty"`

    // Linux capabilities. Default is drop ALL.
    CapAdd  []string `toml:"capAdd"  json:"capAdd,omitempty"`
    CapDrop []string `toml:"capDrop" json:"capDrop,omitempty"` // ["ALL"] by default

    // NoNewPrivileges prevents the container from gaining privileges via setuid.
    // Default true; cannot be disabled unless AllowRoot is also true.
    NoNewPrivileges *bool `toml:"noNewPrivileges" json:"noNewPrivileges,omitempty"`

    // ReadOnlyRootFS mounts / as read-only. Default false (too disruptive for v0),
    // but flagged in dashboard as "hardening recommended."
    ReadOnlyRootFS bool `toml:"readOnlyRootFs" json:"readOnlyRootFs,omitempty"`

    // Seccomp + AppArmor profiles. Empty = runtime default; "" is acceptable.
    SeccompProfile  string `toml:"seccompProfile"  json:"seccompProfile,omitempty"`
    AppArmorProfile string `toml:"appArmorProfile" json:"appArmorProfile,omitempty"`
}

// Default returns the constitution-compliant default profile.
func Default() SecurityProfile {
    t := true
    return SecurityProfile{
        User:            "",           // runtime selects non-root
        AllowRoot:       false,
        CapAdd:          nil,
        CapDrop:         []string{"ALL"},
        NoNewPrivileges: &t,
        ReadOnlyRootFS:  false,
    }
}

// Apply fills zero-valued fields with Default() values without overriding
// explicit caller choices. Implementations of Runtime call this before
// handing the spec to the container backend.
func Apply(p SecurityProfile) SecurityProfile {
    d := Default()
    if p.CapDrop == nil { p.CapDrop = d.CapDrop }
    if p.NoNewPrivileges == nil { p.NoNewPrivileges = d.NoNewPrivileges }
    // User stays "" if caller did not set; runtime picks non-root.
    return p
}

// Validate enforces constitution §II at the API boundary.
// Called during TaskDef parsing; returns an error a user can fix.
func Validate(p SecurityProfile) error {
    if (p.User == "root" || p.User == "0" || p.User == "0:0") && !p.AllowRoot {
        return errors.New("security: user 'root' requires allowRoot=true")
    }
    if p.NoNewPrivileges != nil && !*p.NoNewPrivileges && !p.AllowRoot {
        return errors.New("security: noNewPrivileges=false requires allowRoot=true")
    }
    return nil
}
```

## Behavioral contract

1. **Zero value is safe.** A `SecurityProfile{}` literal yields a fully-locked-down container after `Apply()`. This is what makes constitution §II "Security by Default" enforceable.
2. **Escaping defaults is explicit and auditable.** Any TaskDef that runs as root or disables `NoNewPrivileges` must say `allowRoot = true` in its TOML — visible at PR review and dashboard time.
3. **Runtime MUST call `Apply()` before submitting to the container backend.** This is documented in `runtime.md` step 3 of the behavioral contract.
4. **No way to bypass at the interface boundary.** There is no `Unsafe*` field. If a future workload genuinely needs to be unconfined, it goes through a `Privileged bool` field guarded by `AllowRoot=true`, added in a later feature with its own constitutional review.

## Foundation deliverable

`internal/security/profile.go` ships with:
- The `SecurityProfile` struct.
- `Default()`, `Apply()`, `Validate()` functions, each fully implemented (not stubs).
- A table-driven test verifying that the zero value passes `Validate()` and that `Apply()` produces the constitution-mandated defaults.

This is the **only file in this feature that ships real logic** (rather than `ErrNotImplemented` stubs), because the defaults *are* the contract and are needed by every other feature.
