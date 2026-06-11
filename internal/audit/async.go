package audit

import (
	"context"
	"sync"
	"time"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// AsyncQueueCapacity is the buffered audit enqueue channel size for hot-path writes.
const AsyncQueueCapacity = 1024

const asyncDrainTimeout = 5 * time.Second

var (
	asyncMu  sync.Mutex
	asyncCh  chan storage.AuditEntry
	asyncLog zerolog.Logger
	asyncWg  sync.WaitGroup
)

// StartAsyncWriter starts the background audit writer once per process (no-op if already running).
func StartAsyncWriter(ctx context.Context, st storage.MetaStore, log zerolog.Logger) {
	_ = ctx
	asyncMu.Lock()
	defer asyncMu.Unlock()
	if asyncCh != nil {
		return
	}
	ch := make(chan storage.AuditEntry, AsyncQueueCapacity)
	asyncLog = log
	asyncCh = ch
	asyncWg.Add(1)
	go func() {
		defer asyncWg.Done()
		for entry := range ch {
			if err := st.SaveAuditEntry(context.Background(), entry); err != nil {
				asyncLog.Error().Err(err).Str("action", entry.Action).Msg("async audit save failed")
			}
		}
	}()
}

// Enqueue schedules a hot-path audit write without blocking the caller.
// When the queue is full the entry is dropped and a warning is logged.
func Enqueue(entry storage.AuditEntry) {
	asyncMu.Lock()
	ch := asyncCh
	log := asyncLog
	asyncMu.Unlock()
	if ch == nil {
		if log.GetLevel() != zerolog.Disabled {
			log.Warn().Str("action", entry.Action).Msg("async audit writer not started; dropping entry")
		}
		return
	}
	select {
	case ch <- entry:
	default:
		log.Warn().Str("action", entry.Action).Msg("async audit queue full; dropping entry")
	}
}

// StopAsyncWriter closes the queue and waits for the worker to drain (best-effort, with timeout).
func StopAsyncWriter() {
	asyncMu.Lock()
	ch := asyncCh
	asyncCh = nil
	asyncMu.Unlock()
	if ch == nil {
		return
	}
	close(ch)
	done := make(chan struct{})
	go func() {
		asyncWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(asyncDrainTimeout):
		asyncLog.Warn().Msg("async audit writer drain timed out")
	}
}
