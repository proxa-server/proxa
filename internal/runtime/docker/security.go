package docker

import (
	"fmt"

	"github.com/docker/docker/api/types/container"

	"github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/security"
)

// applySecurityProfile maps a runtime.ContainerSpec to Docker's
// container.Config + container.HostConfig, enforcing the §II security
// defaults. This is the single chokepoint that guarantees every
// Proxa-managed container is non-root + cap-dropped + no-new-privs
// regardless of caller discipline.
//
// FR-002 default: when security.User is empty AND AllowRoot is false,
// User is forced to "1000:1000". Without this, Docker would honor the
// image's USER directive — which is `root` for many common images
// (nginx:alpine, redis, etc.).
func applySecurityProfile(spec runtime.ContainerSpec) (cfg *container.Config, host *container.HostConfig) {
	prof := security.Apply(spec.Security)

	cfg = &container.Config{
		Image:  spec.Image,
		Env:    envSlice(spec.Env),
		Cmd:    spec.Cmd,
		Labels: nil, // populated by caller via BuildContainerLabels
		User:   resolveUser(prof),
	}

	host = &container.HostConfig{
		CapAdd:         prof.CapAdd,
		CapDrop:        prof.CapDrop,
		ReadonlyRootfs: prof.ReadOnlyRootFS,
		SecurityOpt:    securityOpts(prof),
	}

	return cfg, host
}

// resolveUser implements the FR-002 default: empty user + !AllowRoot
// produces "1000:1000". An explicit user is honored verbatim.
func resolveUser(p security.SecurityProfile) string {
	if p.User != "" {
		return p.User
	}
	if p.AllowRoot {
		return "" // let Docker honor the image's USER directive
	}
	return "1000:1000"
}

// securityOpts builds the SecurityOpt slice. Always includes
// no-new-privileges:true unless explicitly disabled (which itself
// requires AllowRoot=true at validation time).
func securityOpts(p security.SecurityProfile) []string {
	var opts []string
	if p.NoNewPrivileges == nil || *p.NoNewPrivileges {
		opts = append(opts, "no-new-privileges:true")
	}
	if p.SeccompProfile != "" {
		opts = append(opts, fmt.Sprintf("seccomp=%s", p.SeccompProfile))
	}
	if p.AppArmorProfile != "" {
		opts = append(opts, fmt.Sprintf("apparmor=%s", p.AppArmorProfile))
	}
	return opts
}

// envSlice converts map env to docker's KEY=VALUE slice form.
func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}
