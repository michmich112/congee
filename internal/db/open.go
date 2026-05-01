package db

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/postgres"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

// Handle is a configured Store with optional cross-instance notifier and shutdown.
type Handle struct {
	storage.Store
	storage.EventNotifier
	closeFn func() error
}

// Close releases database and notifier resources.
func (h *Handle) Close() error {
	if h.closeFn == nil {
		return nil
	}
	return h.closeFn()
}

// Open opens the database from JSON config (SQLite or PostgreSQL).
// log is passed to the store implementation for optional connector debug (use zerolog.Nop() when silent).
func Open(ctx context.Context, sec config.DatabaseSection, log zerolog.Logger) (*Handle, error) {
	switch sec.Type {
	case "", "sqlite":
		st, err := sqlite.Open(ctx, sec.DSN, nil, log)
		if err != nil {
			return nil, err
		}
		return &Handle{
			Store:         st,
			EventNotifier: storage.NoopNotifier{},
			closeFn:       st.Close,
		}, nil
	case "postgres":
		st, err := postgres.Open(ctx, sec.DSN, log)
		if err != nil {
			return nil, err
		}
		return &Handle{
			Store:         st,
			EventNotifier: st.Notifier(),
			closeFn:       st.Close,
		}, nil
	default:
		return nil, fmt.Errorf("db: unsupported database.type %q", sec.Type)
	}
}
