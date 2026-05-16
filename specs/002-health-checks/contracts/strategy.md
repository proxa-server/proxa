# Contract: `internal/reconciler.Strategy`

Deploy-strategy abstraction. Two implementations in v0.2: `start-first` (stateless default) and `stop-first` (stateful default). Future canary / blue-green plug in here.

## Go signature

```go
package reconciler

import (
    "context"

    "github.com/proxa-server/proxa/internal/probe"
    "github.com/proxa-server/proxa/internal/runtime"
    "github.com/proxa-server/proxa/pkg/types"
)

// Strategy executes a per-replica replacement (old container ID known,
// new spec known) according to the strategy's safety contract.
type Strategy interface {
    // Name identifies the strategy ("start-first", "stop-first").
    Name() string

    // Apply replaces the replica at index `replicaIdx` for the given
    // service. The new container's spec_hash will be `newSpec`'s hash.
    // Returns nil on success; ErrRolledBack if the new container failed
    // probes and the old was retained; any other error means partial
    // failure (operator + next reconcile tick will retry).
    Apply(ctx context.Context, req Request) error
}

// Request is the per-replacement input to Strategy.Apply.
type Request struct {
    Project    string
    Service    string
    ReplicaIdx int
    OldID      string          // "" if no old container (fresh create)
    NewSpec    types.TaskDef   // includes Health for probe gating
    Runtime    runtime.Runtime
    Probes     *probe.Manager
}

// ErrRolledBack signals a start-first replacement was reverted because
// the new container failed every probe within the deadline. The old
// container is still running.
var ErrRolledBack = errors.New("reconciler: replacement rolled back")
```

## StartFirst behavior (stateless default)

1. Pull image (`Runtime.PullImage`).
2. Create the new container (`Runtime.CreateContainer`) with the same name suffix as the old (Docker collision avoidance: temporary name like `proxa-{project}-{service}-{replica}-new`).
3. Start the new container (`Runtime.StartContainer`).
4. Track the new container with the probe manager (`Probes.Track(newID, spec.Health)`).
5. Wait up to `interval × retries` for the new container's first healthy snapshot.
6. **If healthy**: rename old container, remove old container (`Runtime.RemoveContainer(oldID, force=true)`), rename new container to the canonical replica name. Untrack old.
7. **If not healthy**: untrack new, remove new container, leave old running. Return `ErrRolledBack` and log WARN with last probe error.

Step 6 has a small race: between "remove old" and "rename new" the canonical name is unbound. v0.2 accepts this as a ≤100ms gap that no one will hit. Future polish: use `proxa-{project}-{service}-{replica}-v{N}` rolling names.

## StopFirst behavior (stateful default)

1. Stop old container with `gracePeriod = 30s`.
2. Wait up to `gracePeriod + 5s` for the container to fully exit (poll `Runtime.InspectContainer.State == "exited"`).
3. Remove old container.
4. Untrack old from probe manager.
5. Pull image, create new container with the canonical name (now free), start it.
6. Track new with probe manager.
7. Wait up to `interval × retries` for the new container's first healthy snapshot.
8. **If healthy**: return nil.
9. **If not healthy**: log WARN. Container stays — the reconciler's next tick will see actual replicas mismatch and decide. (Stop-first does NOT roll back; the old data could already be migrated.)

The asymmetry is intentional: stateful workloads can't be casually undone, so stop-first's "rollback" is "next reconcile tick's call".

## Selection

```go
// SelectStrategy returns the right Strategy for a TaskDef.
// Honors explicit spec.Strategy; falls back to spec.Stateful default.
func SelectStrategy(spec types.TaskDef) Strategy {
    switch spec.Strategy {
    case types.StrategyStartFirst:
        return &StartFirst{}
    case types.StrategyStopFirst:
        return &StopFirst{}
    case "":
        if spec.Stateful {
            return &StopFirst{}
        }
        return &StartFirst{}
    }
    return &StartFirst{} // safe default
}
```

## What's not in v0.2

- Canary (`canary-percent`).
- Blue-green (full-environment swap).
- Strategy-specific options (e.g., StartFirst's "max parallel replacements").
- Pre-stop hooks / lifecycle commands.
