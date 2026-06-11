package db

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

const (
	sqliteAnalyzeInterval = time.Hour
	// Large event DBs can take a long time; cancel only on shutdown or this cap.
	sqliteAnalyzeTimeout = 2 * time.Hour
)

// sqliteAnalyzeInitial delays the first analyze after process start (overridable in tests).
var sqliteAnalyzeInitial = time.Minute

type sqliteStatsAnalyzer struct {
	label string
	run   func(context.Context) error
}

// StartSQLiteAnalyzeLoop periodically runs ANALYZE on SQLite databases used for admin stats.
// The loop stops when ctx is cancelled (e.g. on database Close).
func StartSQLiteAnalyzeLoop(ctx context.Context, analyzers []sqliteStatsAnalyzer, log zerolog.Logger) {
	if len(analyzers) == 0 {
		return
	}
	log = log.With().Str("component", "sqlite_analyze").Logger()
	go func() {
		runAll := func(trigger string) {
			for _, a := range analyzers {
				runCtx, cancel := context.WithTimeout(context.Background(), sqliteAnalyzeTimeout)
				started := time.Now()
				err := a.run(runCtx)
				cancel()
				evt := log.Info().Str("db", a.label).Str("trigger", trigger).Dur("duration_ms", time.Since(started))
				if err != nil {
					log.Error().Err(err).Str("db", a.label).Str("trigger", trigger).Dur("duration_ms", time.Since(started)).Msg("sqlite analyze failed")
					continue
				}
				evt.Msg("sqlite analyze completed")
			}
		}

		initial := time.NewTimer(sqliteAnalyzeInitial)
		defer initial.Stop()
		tick := time.NewTicker(sqliteAnalyzeInterval)
		defer tick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-initial.C:
				runAll("initial")
			case <-tick.C:
				runAll("hourly")
			}
		}
	}()
}
