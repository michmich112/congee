package nip77

import (
	"testing"

	"github.com/michmich112/congee/internal/storage"
)

func TestBuildVectorReconcileRoundTrip(t *testing.T) {
	items := []storage.SyncItem{
		{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: 100},
		{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", CreatedAt: 200},
		{ID: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", CreatedAt: 300},
	}
	server := NewServerNegentropy(BuildVector(items), 1<<20)

	client := NewClientNegentropy(BuildVector(items[:2]), 1<<20)
	clientStart := client.Start()

	out, err := server.Reconcile(clientStart)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("expected server response")
	}

	out2, err := client.Reconcile(out)
	if err != nil {
		t.Fatal(err)
	}
	_ = out2
}

func TestBuildVectorEmpty(t *testing.T) {
	vec := BuildVector(nil)
	if vec.Size() != 0 {
		t.Fatalf("want 0 items, got %d", vec.Size())
	}
	neg := NewServerNegentropy(vec, 1<<20)
	client := NewClientNegentropy(BuildVector([]storage.SyncItem{
		{ID: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", CreatedAt: 1},
	}), 1<<20)
	out, err := neg.Reconcile(client.Start())
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("expected response for empty server set")
	}
}
