package relay

import (
	"context"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// RunImportedEventFanout loads events by id from NOTIFY/LISTEN and broadcasts to REQ subscriptions.
// It returns when ctx is done or the notifier channel is closed.
func RunImportedEventFanout(ctx context.Context, s *Server, store storage.Store, n storage.EventNotifier, log zerolog.Logger) {
	if n == nil {
		n = storage.NoopNotifier{}
	}
	ch := n.Listen()
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-ch:
			if !ok {
				return
			}
			if id == "" {
				continue
			}
			qctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			evs, err := store.QueryEvents(qctx, []nostr.Filter{{IDs: []string{id}}})
			cancel()
			if err != nil || len(evs) != 1 {
				log.Debug().Err(err).Str("event_id", id).Msg("imported event fetch skipped")
				continue
			}
			s.broadcastEvent(evs[0])
		}
	}
}
