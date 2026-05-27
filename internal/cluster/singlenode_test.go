package cluster_test

import (
	"context"
	"errors"
	"testing"

	"github.com/proxa-server/proxa/internal/cluster"
)

func TestSingleNode_Self_ReturnsLocal(t *testing.T) {
	n := cluster.NewSingleNode("node-local", "127.0.0.1:8080", map[string]string{"role": "control"})
	got, err := n.Self(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "node-local" {
		t.Errorf("ID = %q, want node-local", got.ID)
	}
	if got.Labels["role"] != "control" {
		t.Errorf("Labels = %v, want role=control", got.Labels)
	}
}

func TestSingleNode_List_ContainsOnlySelf(t *testing.T) {
	n := cluster.NewSingleNode("n1", "", nil)
	got, err := n.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len(List) = %d, want 1", len(got))
	}
	if got[0].ID != "n1" {
		t.Errorf("List[0].ID = %q, want n1", got[0].ID)
	}
}

func TestSingleNode_StateStore_GetPutDelete(t *testing.T) {
	n := cluster.NewSingleNode("n1", "", nil)
	ctx := context.Background()

	// Get missing → ErrKeyNotFound.
	if _, err := n.Get(ctx, "missing"); !errors.Is(err, cluster.ErrKeyNotFound) {
		t.Errorf("Get missing err = %v, want ErrKeyNotFound", err)
	}

	// Put + Get round-trip.
	if err := n.Put(ctx, "foo", []byte("bar")); err != nil {
		t.Fatal(err)
	}
	got, err := n.Get(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bar" {
		t.Errorf("Get = %q, want bar", got)
	}

	// Delete + Get → not found.
	if err := n.Delete(ctx, "foo"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Get(ctx, "foo"); !errors.Is(err, cluster.ErrKeyNotFound) {
		t.Errorf("post-delete Get err = %v, want ErrKeyNotFound", err)
	}
}

func TestSingleNode_Scheduler_AlwaysSelf(t *testing.T) {
	n := cluster.NewSingleNode("only-node", "", nil)
	id, err := n.Place(context.Background(), "task-1", map[string]string{"labels.region": "elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "only-node" {
		t.Errorf("Place = %q, want only-node (single-node mode ignores constraints)", id)
	}
}

func TestSingleNode_Subscribe_ClosesOnContextCancel(t *testing.T) {
	n := cluster.NewSingleNode("n1", "", nil)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := n.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, open := <-ch; open {
		t.Errorf("expected channel closed after ctx cancel")
	}
}
