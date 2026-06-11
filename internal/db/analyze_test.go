package db

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestStartSQLiteAnalyzeLoopRunsAndStops(t *testing.T) {
	prev := sqliteAnalyzeInitial
	sqliteAnalyzeInitial = 20 * time.Millisecond
	defer func() { sqliteAnalyzeInitial = prev }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runs atomic.Int32
	done := make(chan struct{})
	StartSQLiteAnalyzeLoop(ctx, []sqliteStatsAnalyzer{
		{
			label: "test",
			run: func(context.Context) error {
				if runs.Add(1) == 1 {
					close(done)
				}
				return nil
			},
		},
	}, zerolog.Nop())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial analyze")
	}
	cancel()
}
