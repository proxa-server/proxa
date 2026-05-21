// Package docker is the Docker-Engine implementation of
// [github.com/proxa-server/proxa/internal/runtime.Runtime].
//
// Constitutional notes:
//
//   - §II Security by Default: every CreateContainer call routes through
//     [internal/security.Apply] before submitting to the daemon.
//   - §III Project scoping: ListContainers requires a non-empty project filter.
//
// See specs/000-foundation/contracts/runtime.md for the full contract;
// specs/001-core-loop/data-model.md for the label conventions.
package docker

import (
	"fmt"
	"maps"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
)

// Label keys identifying Proxa-managed containers.
const (
	LabelManaged   = "proxa.managed"
	LabelProject   = "proxa.project"
	LabelService   = "proxa.service"
	LabelReplica   = "proxa.replica"
	LabelSpecHash  = "proxa.spec_hash"
	LabelNodeID    = "proxa.node"
	LabelCreatedAt = "proxa.created_at"
)

// BuildContainerLabels constructs the full label map for a container
// being created. Caller-supplied labels in spec.Labels are preserved
// (but cannot override the Proxa-reserved keys above — those are
// always overwritten with authoritative values).
func BuildContainerLabels(spec runtime.ContainerSpec, replica int, specHash, nodeID string) map[string]string {
	out := map[string]string{}
	maps.Copy(out, spec.Labels)
	out[LabelManaged] = "true"
	out[LabelProject] = spec.Labels[LabelProject] // set by reconciler in spec.Labels
	out[LabelService] = spec.Labels[LabelService]
	out[LabelReplica] = fmt.Sprintf("%d", replica)
	out[LabelSpecHash] = specHash
	out[LabelNodeID] = nodeID
	out[LabelCreatedAt] = time.Now().UTC().Format(time.RFC3339)
	return out
}

// ContainerNameFor returns the deterministic container name for a
// given (project, service, replica) tuple. Format mandated by FR-015:
//
//	proxa-{project}-{service}-{replica}
//
// Docker enforces a 253-char name limit. Project + service slugs are
// each ≤63 chars per the project naming regex, so the worst case is
// 6 + 63 + 1 + 63 + 1 + 6 ≈ 140 chars — comfortably under the limit.
func ContainerNameFor(project, service string, replica int) string {
	return fmt.Sprintf("proxa-%s-%s-%d", project, service, replica)
}
