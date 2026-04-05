package audit

import (
	"context"
	"time"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// StartRetentionLoop periodically purges audit rows older than retentionDays.
func StartRetentionLoop(ctx context.Context, store storage.Store, retentionDays int, log zerolog.Logger) {
	if retentionDays <= 0 {
		return
	}
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
				n, err := store.PurgeAuditLog(context.Background(), cutoff)
				if err != nil {
					log.Error().Err(err).Msg("audit purge failed")
					continue
				}
				if n > 0 {
					log.Info().Int64("rows", n).Msg("audit purge completed")
				}
			}
		}
	}()
}
