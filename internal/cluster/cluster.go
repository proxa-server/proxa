// Package cluster holds the multi-node abstractions Proxa uses to
// coordinate work across hosts. v0.4.3 ships the interfaces + a
// single-node stub implementation; v0.5 Multi-host MVP swaps in a
// central-SQLite implementation; v1.0 swaps in embedded etcd +
// agent gossip.
//
// Locking the interfaces in v0.4.3 lets v0.5 + v1.0 ship without
// re-shaping the consumers (reconciler, scheduler, server endpoints).
package cluster

import (
	"context"
	"time"
)

// Node is one cluster member. In single-node mode the local node is
// returned by Self() and List().
type Node struct {
	ID       string            `json:"id"`
	Addr     string            `json:"addr"`
	Labels   map[string]string `json:"labels"`
	LastSeen time.Time         `json:"last_seen"`
}

// MembershipEventKind describes a Membership change.
type MembershipEventKind string

const (
	MembershipNodeAdded   MembershipEventKind = "node_added"
	MembershipNodeRemoved MembershipEventKind = "node_removed"
	MembershipNodeUpdated MembershipEventKind = "node_updated"
)

// MembershipEvent is one cluster-membership change.
type MembershipEvent struct {
	Kind MembershipEventKind `json:"kind"`
	Node Node                `json:"node"`
}

// Membership reports who's in the cluster. Single-node mode returns
// [self] for List + a never-firing Subscribe channel.
type Membership interface {
	Self(ctx context.Context) (Node, error)
	List(ctx context.Context) ([]Node, error)
	Subscribe(ctx context.Context) (<-chan MembershipEvent, error)
}

// WatchEventKind is the type of cluster-state KV change.
type WatchEventKind string

const (
	WatchPut    WatchEventKind = "put"
	WatchDelete WatchEventKind = "delete"
)

// WatchEvent is one cluster-state KV change.
type WatchEvent struct {
	Kind  WatchEventKind `json:"kind"`
	Key   string         `json:"key"`
	Value []byte         `json:"value,omitempty"`
}

// StateStore is the cluster-wide key-value store. Single-node mode
// uses local SQLite; v0.5 may use central SQLite; v1.0 uses etcd.
type StateStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error)
}

// Scheduler places tasks on cluster nodes. Single-node mode always
// returns the local node; v0.5+ considers node labels + constraints.
type Scheduler interface {
	// Place returns the ID of the node selected for the given task.
	// constraints is a string-string map of operator-supplied
	// requirements (e.g., "labels.region": "us-east").
	Place(ctx context.Context, taskID string, constraints map[string]string) (string, error)
}
