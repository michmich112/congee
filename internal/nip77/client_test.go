package nip77

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/storage"
)

func TestSyncClientManyIDs(t *testing.T) {
	const n = 200
	remote := make([]storage.SyncItem, n)
	for i := 0; i < n; i++ {
		remote[i] = storage.SyncItem{ID: fmt.Sprintf("%064x", i+1), CreatedAt: int64(i + 1)}
	}
	server := NewServerNegentropy(BuildVector(remote), 1<<20)
	client := NewSyncClient(BuildVector(nil), 1<<20)
	defer client.Stop()

	ids := reconcileClient(t, client, server)
	if len(ids) != n {
		t.Fatalf("need %d ids, got %d", n, len(ids))
	}
	got := map[string]struct{}{}
	for _, id := range ids {
		got[id] = struct{}{}
	}
	for _, it := range remote {
		if _, ok := got[it.ID]; !ok {
			t.Fatalf("missing %s", it.ID)
		}
	}
}

func TestSyncClientHaveNotPanicResumes(t *testing.T) {
	const n = 200
	remote := make([]storage.SyncItem, n)
	for i := 0; i < n; i++ {
		remote[i] = storage.SyncItem{ID: fmt.Sprintf("%064x", i+1), CreatedAt: int64(i + 1)}
	}
	var once sync.Once
	var panicked atomicBool
	server := NewServerNegentropy(BuildVector(remote), 1<<20)
	client := NewSyncClient(BuildVector(nil), 1<<20, WithHaveNotObserver(func(string) {
		once.Do(func() {
			panicked.set()
			panic("have-not observer")
		})
	}))
	defer client.Stop()

	ids := reconcileClient(t, client, server)
	if !panicked.get() {
		t.Fatal("expected have-not observer to panic once")
	}
	if len(ids) != n {
		t.Fatalf("need %d ids after observer panic, got %d", n, len(ids))
	}
}

func reconcileClient(t *testing.T, client *SyncClient, server interface {
	Reconcile(string) (string, error)
}) []string {
	t.Helper()
	type result struct {
		ids []string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg := client.Start()
		var err error
		for {
			msg, err = server.Reconcile(msg)
			if err != nil {
				ch <- result{err: err}
				return
			}
			if msg == "" {
				ch <- result{ids: client.NeedIDs()}
				return
			}
			msg, err = client.Reconcile(msg)
			if err != nil {
				ch <- result{err: err}
				return
			}
			if msg == "" {
				ch <- result{ids: client.NeedIDs()}
				return
			}
		}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatal(res.err)
		}
		return res.ids
	case <-time.After(5 * time.Second):
		t.Fatal("reconcile deadlock")
		return nil
	}
}

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *atomicBool) set() {
	b.mu.Lock()
	b.v = true
	b.mu.Unlock()
}

func (b *atomicBool) get() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.v
}
