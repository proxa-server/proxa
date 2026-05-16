// Package hash computes deterministic SHA-256 hashes over types.TaskDef
// via canonical-JSON encoding. Used by store/sqlite for spec_hash
// columns and by reconciler/diff for change detection.
//
// Determinism guarantee: two TaskDef values that semantically describe
// the same workload hash identically across runs, hosts, and Go
// versions. Achieved by JSON-encoding with sorted keys and stable
// numeric formatting.
//
// This is a leaf package — imports stdlib + pkg/types only.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/proxa-server/proxa/pkg/types"
)

// Hash returns "sha256:" + hex(sha256(canonicalJSON(spec))).
//
// Deployment-control fields (Replicas, Strategy, Schedule) are
// intentionally excluded — they describe HOW MANY / WHEN to run, not
// WHAT to run. Scaling replicas up/down or switching deploy strategy
// must NOT trigger a tear-down of existing containers whose
// per-container spec is unchanged.
func Hash(spec types.TaskDef) string {
	stable := spec
	stable.Replicas = 0
	stable.Strategy = ""
	stable.Schedule = ""
	sum := sha256.Sum256(canonicalJSON(stable))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// canonicalJSON encodes a TaskDef with sorted keys at every nesting
// level. Achieved by round-tripping through map[string]any and
// re-encoding via encoding/json (which sorts map keys but not struct
// fields). Then we walk the result, sorting any nested maps.
func canonicalJSON(spec types.TaskDef) []byte {
	raw, err := json.Marshal(spec)
	if err != nil {
		// TaskDef is plain data; json.Marshal cannot fail for it.
		panic("hash: TaskDef encoding failed: " + err.Error())
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		panic("hash: TaskDef round-trip failed: " + err.Error())
	}
	canonical, err := marshalSorted(generic)
	if err != nil {
		panic("hash: canonical encode failed: " + err.Error())
	}
	return canonical
}

// marshalSorted encodes v with all maps' keys sorted recursively.
func marshalSorted(v any) ([]byte, error) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		out := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				out = append(out, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			out = append(out, kb...)
			out = append(out, ':')
			vb, err := marshalSorted(x[k])
			if err != nil {
				return nil, err
			}
			out = append(out, vb...)
		}
		out = append(out, '}')
		return out, nil
	case []any:
		out := []byte{'['}
		for i, item := range x {
			if i > 0 {
				out = append(out, ',')
			}
			ib, err := marshalSorted(item)
			if err != nil {
				return nil, err
			}
			out = append(out, ib...)
		}
		out = append(out, ']')
		return out, nil
	default:
		return json.Marshal(v)
	}
}
