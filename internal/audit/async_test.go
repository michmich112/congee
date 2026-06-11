package audit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

type memMetaStore struct {
	mu           sync.Mutex
	entries      []storage.AuditEntry
	hold         chan struct{}
	holdReady    chan struct{}
	holdReadyOnce sync.Once
}

func (m *memMetaStore) SaveAuditEntry(ctx context.Context, e storage.AuditEntry) error {
	if m.hold != nil {
		m.holdReadyOnce.Do(func() {
			if m.holdReady != nil {
				close(m.holdReady)
			}
		})
		select {
		case <-m.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.mu.Lock()
	m.entries = append(m.entries, e)
	m.mu.Unlock()
	return nil
}

func (m *memMetaStore) HasAuditDuplicate(ctx context.Context, e storage.AuditEntry) (bool, error) {
	_ = ctx
	_ = e
	return false, nil
}

func (m *memMetaStore) QueryAuditLog(ctx context.Context, q storage.AuditQuery) ([]storage.AuditEntry, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []storage.AuditEntry
	for _, e := range m.entries {
		if q.Action != "" && e.Action != q.Action {
			continue
		}
		out = append(out, e)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (m *memMetaStore) CountAuditLog(ctx context.Context, q storage.AuditQuery) (int64, error) {
	rows, err := m.QueryAuditLog(ctx, q)
	return int64(len(rows)), err
}

func (m *memMetaStore) ListDistinctAuditKinds(ctx context.Context, scanLimit int) ([]int, error) {
	_ = ctx
	_ = scanLimit
	return nil, nil
}

func (m *memMetaStore) PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error) {
	_ = ctx
	_ = olderThanUnix
	return 0, nil
}

func (m *memMetaStore) SaveWSConnectionSession(ctx context.Context, s storage.WSConnectionSession) (int64, error) {
	_ = ctx
	_ = s
	return 0, nil
}

func (m *memMetaStore) QueryWSConnectionSessions(ctx context.Context, q storage.WSConnectionSessionQuery) ([]storage.WSConnectionSession, error) {
	_ = ctx
	_ = q
	return nil, nil
}

func (m *memMetaStore) CountWSConnectionSessions(ctx context.Context) (int64, error) {
	_ = ctx
	return 0, nil
}

func (m *memMetaStore) GetWSConnectionSessionByID(ctx context.Context, id int64) (*storage.WSConnectionSession, error) {
	_ = ctx
	_ = id
	return nil, nil
}

func (m *memMetaStore) PurgeWSConnectionSessionsBefore(ctx context.Context, olderThanUnix int64) (int64, error) {
	_ = ctx
	_ = olderThanUnix
	return 0, nil
}

func (m *memMetaStore) SaveConfigChange(ctx context.Context, c storage.ConfigChange) error {
	_ = ctx
	_ = c
	return nil
}

func (m *memMetaStore) QueryConfigChangelog(ctx context.Context, limit int) ([]storage.ConfigChange, error) {
	_ = ctx
	_ = limit
	return nil, nil
}

func (m *memMetaStore) UpsertRelayMetricBucket(ctx context.Context, b storage.RelayMetricBucket) error {
	_ = ctx
	_ = b
	return nil
}

func (m *memMetaStore) QueryRelayMetricBuckets(ctx context.Context, q storage.RelayMetricBucketQuery) ([]storage.RelayMetricBucket, error) {
	_ = ctx
	_ = q
	return nil, nil
}

func (m *memMetaStore) PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error) {
	_ = ctx
	_ = cutoffStartUnixExclusive
	return 0, nil
}

func TestEnqueueNonBlocking(t *testing.T) {
	StopAsyncWriter()
	meta := &memMetaStore{hold: make(chan struct{})}
	ctx := context.Background()
	StartAsyncWriter(ctx, meta, zerolog.Nop())
	defer StopAsyncWriter()

	entry := storage.AuditEntry{CreatedAt: 1, Action: "test", Detail: "d", Pubkey: "pk"}
	Enqueue(entry)

	done := make(chan struct{})
	go func() {
		Enqueue(storage.AuditEntry{CreatedAt: 2, Action: "test2", Detail: "d2", Pubkey: "pk2"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Enqueue blocked caller")
	}
	close(meta.hold)
}

func TestAsyncWorkerPersists(t *testing.T) {
	StopAsyncWriter()
	meta := &memMetaStore{}
	ctx := context.Background()
	StartAsyncWriter(ctx, meta, zerolog.Nop())
	defer StopAsyncWriter()

	want := storage.AuditEntry{CreatedAt: 99, Action: "event_stored", Detail: "x", Pubkey: "abc"}
	Enqueue(want)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		meta.mu.Lock()
		n := len(meta.entries)
		meta.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	meta.mu.Lock()
	defer meta.mu.Unlock()
	if len(meta.entries) != 1 {
		t.Fatalf("entries: %d", len(meta.entries))
	}
	got := meta.entries[0]
	if got.Action != want.Action || got.Detail != want.Detail || got.Pubkey != want.Pubkey {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestEnqueueDropsWhenQueueFull(t *testing.T) {
	StopAsyncWriter()
	meta := &memMetaStore{hold: make(chan struct{}), holdReady: make(chan struct{})}
	ctx := context.Background()
	StartAsyncWriter(ctx, meta, zerolog.Nop())
	defer StopAsyncWriter()

	Enqueue(storage.AuditEntry{CreatedAt: 1, Action: "block", Detail: "d", Pubkey: "pk"})
	select {
	case <-meta.holdReady:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not reach blocked SaveAuditEntry")
	}
	for i := 0; i < AsyncQueueCapacity; i++ {
		Enqueue(storage.AuditEntry{
			CreatedAt: int64(i + 2),
			Action:    "fill",
			Detail:    "d",
			Pubkey:    "pk",
		})
	}
	Enqueue(storage.AuditEntry{CreatedAt: 9999, Action: "overflow", Detail: "d", Pubkey: "pk"})

	close(meta.hold)
	StopAsyncWriter()

	meta.mu.Lock()
	defer meta.mu.Unlock()
	for _, e := range meta.entries {
		if e.Action == "overflow" {
			t.Fatal("overflow entry was persisted")
		}
	}
	if len(meta.entries) != AsyncQueueCapacity+1 {
		t.Fatalf("persisted %d entries, want %d", len(meta.entries), AsyncQueueCapacity+1)
	}
}
