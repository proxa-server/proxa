package reconciler

import (
	"testing"

	"github.com/proxa-server/proxa/internal/probe"
	"github.com/proxa-server/proxa/pkg/types"
)

func snap(id string, ok bool) probe.Snapshot {
	return probe.Snapshot{ContainerID: id, HealthOK: ok}
}

func TestAggregate(t *testing.T) {
	cases := []struct {
		name      string
		desired   int
		snapshots []probe.Snapshot
		want      types.ServiceStatus
	}{
		{"stopped — desired 0 + no replicas", 0, nil, types.ServiceStatusStopped},
		{"healthy — 3/3 all ok", 3, []probe.Snapshot{snap("a", true), snap("b", true), snap("c", true)}, types.ServiceStatusHealthy},
		{"degraded — 2/3 ok", 3, []probe.Snapshot{snap("a", true), snap("b", true), snap("c", false)}, types.ServiceStatusDegraded},
		{"failed — 0/2 ok", 2, []probe.Snapshot{snap("a", false), snap("b", false)}, types.ServiceStatusFailed},
		{"degraded — scale-up in progress (2 healthy, want 3)", 3, []probe.Snapshot{snap("a", true), snap("b", true)}, types.ServiceStatusDegraded},
		{"failed — desired 1, no replicas yet", 1, nil, types.ServiceStatusFailed},
		{"healthy — desired 1, 1 healthy", 1, []probe.Snapshot{snap("a", true)}, types.ServiceStatusHealthy},
		{"failed — desired 1, replica unhealthy", 1, []probe.Snapshot{snap("a", false)}, types.ServiceStatusFailed},
		{"degraded — desired 2, 1 healthy + 1 stale stopped replica still tracked", 2,
			[]probe.Snapshot{snap("a", true), snap("b", false), snap("c", true)}, types.ServiceStatusDegraded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Aggregate(tc.desired, tc.snapshots)
			if got != tc.want {
				t.Errorf("Aggregate(desired=%d, %d snapshots) = %q, want %q",
					tc.desired, len(tc.snapshots), got, tc.want)
			}
		})
	}
}
