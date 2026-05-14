package types

import "time"

// Node is a cluster member — either the control-plane server or a
// per-host agent. Nodes are not project-scoped; they belong to the
// cluster.
type Node struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Role           NodeRole          `json:"role"`
	Address        string            `json:"address"` // host:port reachable from control plane
	Resources      NodeResources     `json:"resources"`
	ContainerCount int               `json:"containerCount"`
	LastHeartbeat  time.Time         `json:"lastHeartbeat"`
	Status         NodeStatus        `json:"status"`
	Labels         map[string]string `json:"labels,omitempty"` // for placement constraints
}

// NodeRole identifies whether a node runs the control plane or only
// the agent. Both are first-class from v0 (constitution §VI).
type NodeRole string

const (
	NodeRoleServer NodeRole = "server"
	NodeRoleAgent  NodeRole = "agent"
)

// NodeStatus is the health of a node from the control plane's perspective.
type NodeStatus string

const (
	NodeStatusReady    NodeStatus = "ready"
	NodeStatusDraining NodeStatus = "draining"
	NodeStatusOffline  NodeStatus = "offline"
)

// NodeResources captures the host's reported capacity. Used by the
// scheduler for placement decisions.
type NodeResources struct {
	CPUCores int   `json:"cpuCores"`
	MemoryMB int64 `json:"memoryMb"`
	DiskGB   int64 `json:"diskGb"`
}
