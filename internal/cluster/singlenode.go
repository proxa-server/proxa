package cluster

import (
	"context"
	"errors"
	"sync"
	"time"
)

// SingleNode is the v0.4.3 stub implementation: one node (self),
// in-memory KV store, all schedule decisions resolve to self.
//
// v0.5 Multi-host MVP replaces SingleNode with CentralSQLite (the
// control plane reads from SQLite while N agents pull-mTLS dial in).
// v1.0 replaces with EmbeddedEtcd.
//
// The Membership, StateStore, and Scheduler interfaces all stay
// stable; consumers (reconciler, server) don't change when the
// implementation swaps.
type SingleNode struct {
	self Node

	mu sync.RWMutex
	kv map[string][]byte
}

// Compile-time assertions.
var (
	_ Membership = (*SingleNode)(nil)
	_ StateStore = (*SingleNode)(nil)
	_ Scheduler  = (*SingleNode)(nil)
)

// NewSingleNode returns a SingleNode rooted at the given node ID.
// addr is informational; labels can be nil.
func NewSingleNode(id, addr string, labels map[string]string) *SingleNode {
	if labels == nil {
		labels = map[string]string{}
	}
	return &SingleNode{
		self: Node{
			ID:       id,
			Addr:     addr,
			Labels:   labels,
			LastSeen: time.Now(),
		},
		kv: map[string][]byte{},
	}
}

// --- Membership ---------------------------------------------------------

func (s *SingleNode) Self(context.Context) (Node, error) {
	return s.self, nil
}

func (s *SingleNode) List(context.Context) ([]Node, error) {
	return []Node{s.self}, nil
}

func (s *SingleNode) Subscribe(ctx context.Context) (<-chan MembershipEvent, error) {
	// Single-node: membership never changes. Return a channel that
	// closes when ctx ends; subscribers can block on it for the
	// process lifetime without leaking.
	ch := make(chan MembershipEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// --- StateStore ---------------------------------------------------------

// ErrKeyNotFound is returned by Get when the key does not exist.
var ErrKeyNotFound = errors.New("cluster: key not found")

func (s *SingleNode) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.kv[key]
	if !ok {
		return nil, ErrKeyNotFound
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (s *SingleNode) Put(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := make([]byte, len(value))
	copy(stored, value)
	s.kv[key] = stored
	return nil
}

func (s *SingleNode) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.kv, key)
	return nil
}

func (s *SingleNode) Watch(ctx context.Context, _ string) (<-chan WatchEvent, error) {
	// Single-node: no remote watchers; return a never-firing channel
	// that closes when ctx ends.
	ch := make(chan WatchEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// --- Scheduler ----------------------------------------------------------

func (s *SingleNode) Place(_ context.Context, _ string, _ map[string]string) (string, error) {
	// Always self in single-node mode.
	return s.self.ID, nil
}
