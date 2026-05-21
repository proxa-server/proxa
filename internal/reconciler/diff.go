// Package reconciler implements the core reconciliation loop that
// drives actual container state toward desired state in the
// StateStore. This is the coordinator from the Complexity Tracking
// deviation in plan.md — concrete code, not behind an interface.
package reconciler

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/proxa-server/proxa/internal/hash"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/pkg/types"
)

// ActionType is the kind of reconciliation action.
type ActionType string

const (
	ActionCreate  ActionType = "create"
	ActionRemove  ActionType = "remove"
	ActionReplace ActionType = "replace"
)

// Action is one reconciliation action computed by Compute.
// Pure data — no goroutines, no I/O. Apply consumes these.
type Action struct {
	Type ActionType

	// For Create + Replace
	Project  string
	Service  string
	Replica  int
	Spec     types.TaskDef
	SpecHash string

	// For Remove + Replace
	ContainerID string

	// Why this action was chosen (logged on apply).
	Reason string
}

// Compute is the pure diff function. Input: snapshot of desired state
// (services from StateStore) + actual state (containers from Runtime)
// + optional probeUnhealthy set (containerIDs whose probe streak has
// hit retries; nil means "no probe input"). Output: ordered list of
// actions to execute. Deterministic ordering by (project, service, replica).
//
// Removal triggers (symmetric — both free the replica slot so the
// desired-side loop generates a fresh Create):
//   - Container in a non-active state (exited / dead / removing / paused)
//     — handles the killed/crashed container case from 001.
//   - Container is probe-unhealthy past its retry budget — handles the
//     workload-stopped-responding case from 002 (FR-006).
func Compute(desired []types.Service, actual []rt.ContainerInfo, probeUnhealthy map[string]bool) []Action {
	type key struct{ project, service string; replica int }
	actualMap := map[key]rt.ContainerInfo{}
	var actions []Action

	for _, c := range actual {
		p := c.Labels[dockerlabels.LabelProject]
		s := c.Labels[dockerlabels.LabelService]
		r, err := strconv.Atoi(c.Labels[dockerlabels.LabelReplica])
		if err != nil {
			continue // skip non-conforming containers
		}
		dead := !isActive(c.State)
		unhealthy := probeUnhealthy[c.ID]
		if !dead && !unhealthy {
			actualMap[key{p, s, r}] = c
			continue
		}
		reason := "container in non-running state: " + c.State
		if !dead && unhealthy {
			reason = "probe streak exceeded retries"
		}
		actions = append(actions, Action{
			Type:        ActionRemove,
			Project:     p,
			Service:     s,
			Replica:     r,
			ContainerID: c.ID,
			Reason:      reason,
		})
	}

	// Build desired set + per-service spec hash.
	type desiredKey = key
	desiredSet := map[desiredKey]types.Service{}
	for _, svc := range desired {
		for r := 0; r < svc.Spec.Replicas; r++ {
			desiredSet[desiredKey{svc.Project, svc.Name, r}] = svc
		}
	}

	// Pass 1: handle desired entries — create or replace.
	for k, svc := range desiredSet {
		want := hash.Hash(svc.Spec)
		c, exists := actualMap[k]
		if !exists {
			actions = append(actions, Action{
				Type:     ActionCreate,
				Project:  k.project,
				Service:  k.service,
				Replica:  k.replica,
				Spec:     svc.Spec,
				SpecHash: want,
				Reason:   "actual<desired",
			})
			continue
		}
		got := c.Labels[dockerlabels.LabelSpecHash]
		if got != want {
			actions = append(actions, Action{
				Type:        ActionReplace,
				Project:     k.project,
				Service:     k.service,
				Replica:     k.replica,
				Spec:        svc.Spec,
				SpecHash:    want,
				ContainerID: c.ID,
				Reason:      "spec_hash drift",
			})
		}
	}

	// Pass 2: handle actual entries with no desired match — remove.
	for k, c := range actualMap {
		if _, wanted := desiredSet[k]; !wanted {
			actions = append(actions, Action{
				Type:        ActionRemove,
				Project:     k.project,
				Service:     k.service,
				Replica:     k.replica,
				ContainerID: c.ID,
				Reason:      "actual>desired",
			})
		}
	}

	slices.SortFunc(actions, func(a, b Action) int {
		if c := cmp.Compare(a.Project, b.Project); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Service, b.Service); c != 0 {
			return c
		}
		return cmp.Compare(a.Replica, b.Replica)
	})

	return actions
}

// isActive reports whether a container in the given Docker state is
// counted as fulfilling its replica slot. Exited/dead/removing/paused
// containers are NOT active — the reconciler removes them and recreates.
func isActive(state string) bool {
	switch state {
	case "running", "restarting", "created":
		return true
	}
	return false
}
