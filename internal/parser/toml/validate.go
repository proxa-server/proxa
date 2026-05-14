package toml

import (
	"fmt"
	"regexp"

	"github.com/proxa-server/proxa/internal/security"
	"github.com/proxa-server/proxa/pkg/types"
)

// Each error code below is stable and matches contracts/toml-grammar.md.
type validationError struct {
	Code    string
	Message string
}

func (e *validationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Code returns the stable error code, so the CLI can prefix user output.
func (e *validationError) ErrCode() string { return e.Code }

var (
	nameRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	cpuRe      = regexp.MustCompile(`^\d+m?$`)
	memoryRe   = regexp.MustCompile(`^\d+(Ki|Mi|Gi)?$`)
	allowedProtocols = map[string]bool{"tcp": true, "udp": true, "http": true, "https": true}
	allowedStrategies = map[types.DeployStrategy]bool{
		types.StrategyStartFirst: true,
		types.StrategyStopFirst:  true,
		"":                      true, // optional; default applied later
	}
)

// Validate checks every rule from contracts/toml-grammar.md and returns
// the first violation with its stable error code.
func Validate(td types.TaskDef) error {
	if !nameRe.MatchString(td.Project) {
		return &validationError{Code: "invalid-project-name", Message: fmt.Sprintf("project %q does not match ^[a-z0-9][a-z0-9-]{0,62}$", td.Project)}
	}
	if !nameRe.MatchString(td.Name) {
		return &validationError{Code: "missing-or-invalid-name", Message: fmt.Sprintf("name %q is required and must match ^[a-z0-9][a-z0-9-]{0,62}$", td.Name)}
	}
	if td.Image == "" {
		return &validationError{Code: "invalid-image-ref", Message: "image is required"}
	}
	if td.Replicas < 0 {
		return &validationError{Code: "invalid-replicas", Message: fmt.Sprintf("replicas must be >= 0, got %d", td.Replicas)}
	}
	if !allowedStrategies[td.Strategy] {
		return &validationError{Code: "invalid-strategy", Message: fmt.Sprintf("strategy %q is not one of start-first|stop-first", td.Strategy)}
	}
	if err := security.Validate(td.Security); err != nil {
		// Map to the appropriate grammar code based on the error.
		msg := err.Error()
		if contains(msg, "user 'root' requires") {
			return &validationError{Code: "root-requires-allowroot", Message: msg}
		}
		if contains(msg, "noNewPrivileges=false requires") {
			return &validationError{Code: "nonewprivileges-requires-allowroot", Message: msg}
		}
		return &validationError{Code: "invalid-security", Message: msg}
	}
	for i, p := range td.Expose {
		if !allowedProtocols[p.Protocol] {
			return &validationError{Code: "invalid-protocol", Message: fmt.Sprintf("expose[%d].protocol %q must be tcp|udp|http|https", i, p.Protocol)}
		}
		if p.Container < 1 || p.Container > 65535 {
			return &validationError{Code: "invalid-port", Message: fmt.Sprintf("expose[%d].container %d out of 1..65535", i, p.Container)}
		}
		if p.Host < 0 || p.Host > 65535 {
			return &validationError{Code: "invalid-port", Message: fmt.Sprintf("expose[%d].host %d out of 0..65535", i, p.Host)}
		}
	}
	for i, v := range td.Volumes {
		if v.Source == "" {
			return &validationError{Code: "invalid-volume-source", Message: fmt.Sprintf("volumes[%d].source is required", i)}
		}
	}
	if td.Resources.CPU != "" && !cpuRe.MatchString(td.Resources.CPU) {
		return &validationError{Code: "invalid-resource", Message: fmt.Sprintf("resources.cpu %q must match ^\\d+m?$", td.Resources.CPU)}
	}
	if td.Resources.Memory != "" && !memoryRe.MatchString(td.Resources.Memory) {
		return &validationError{Code: "invalid-resource", Message: fmt.Sprintf("resources.memory %q must match ^\\d+(Ki|Mi|Gi)?$", td.Resources.Memory)}
	}
	// schedule cron validation is shallow in v0.0 — accept any non-empty string;
	// full cron parser arrives with the scheduler in Feature 002.
	return nil
}

// contains is a tiny strings.Contains alternative.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
