package types

import "time"

// Service is a running stateless or stateful workload tracked in the
// state store. The Spec field is the last successfully applied [TaskDef].
type Service struct {
	ID        string             `json:"id"`
	Project   string             `json:"project"`
	Name      string             `json:"name"`
	Spec      TaskDef            `json:"spec"`
	Status    ServiceStatus      `json:"status"`
	Replicas  []ReplicaState     `json:"replicas"`
	History   []DeploymentRecord `json:"history"` // capped; oldest dropped
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

// ServiceStatus is the aggregate health derived from replica states.
// Values:
//   - pending     : service just upserted, reconciler hasn't ticked yet
//   - reconciling : actual replicas != desired (mid-rollover)
//   - healthy     : actual == desired AND every replica's probe passes
//   - degraded    : actual == desired BUT at least one probe is failing
//   - failed      : actual < desired AND no healthy replicas (image broken?)
//   - stopped     : desired == 0 AND actual == 0 (proxa down)
type ServiceStatus string

const (
	ServiceStatusPending     ServiceStatus = "pending"
	ServiceStatusReconciling ServiceStatus = "reconciling"
	ServiceStatusHealthy     ServiceStatus = "healthy"
	ServiceStatusDegraded    ServiceStatus = "degraded"
	ServiceStatusFailed      ServiceStatus = "failed"
	ServiceStatusStopped     ServiceStatus = "stopped"
)

// ReplicaState is one container's runtime state, as observed by the
// reconciler.
type ReplicaState struct {
	ID          string    `json:"id"`     // container ID from the runtime
	NodeID      string    `json:"nodeId"`
	Phase       string    `json:"phase"`  // starting | running | exiting | failed
	HealthOK    bool      `json:"healthOk"`
	StartedAt   time.Time `json:"startedAt"`
	LastProbeAt time.Time `json:"lastProbeAt"`
}

// DeploymentRecord is one entry in a service's deployment history.
// The full Spec is snapshotted so rollback is instant — no need to
// re-derive from external sources.
type DeploymentRecord struct {
	DeployedAt time.Time `json:"deployedAt"`
	Image      string    `json:"image"`
	Spec       TaskDef   `json:"spec"`
	Outcome    string    `json:"outcome"` // success | failed | rolled-back
}
