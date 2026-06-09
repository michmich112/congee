package db

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/postgres"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/michmich112/congee/internal/storage/sqlitemeta"
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
// relayInstanceID is the PostgreSQL LISTEN/NOTIFY origin id (ignored for SQLite).
// log is passed to the store implementation for optional connector debug (use zerolog.Nop() when silent).
func Open(ctx context.Context, sec config.DatabaseSection, relayInstanceID string, log zerolog.Logger) (*Handle, error) {
	switch sec.Type {
	case "", "sqlite":
		return openSQLite(ctx, sec, log)
	case "postgres":
		metaDSN := ResolveMetaDSN(sec)
		meta, err := sqlitemeta.Open(ctx, metaDSN, log)
		if err != nil {
			return nil, err
		}
		if err := migrateLegacyMetaPostgres(ctx, sec.DSN, meta); err != nil {
			_ = meta.Close()
			return nil, fmt.Errorf("db: legacy meta migration: %w", err)
		}
		st, err := postgres.Open(ctx, sec.DSN, relayInstanceID, log)
		if err != nil {
			_ = meta.Close()
			return nil, err
		}
		store := newCompositeStore(st, meta, st, meta)
		return &Handle{
			Store:         store,
			EventNotifier: st.Notifier(),
			closeFn: func() error {
				err1 := meta.Close()
				err2 := st.Close()
				if err1 != nil {
					return err1
				}
				return err2
			},
		}, nil
	default:
		return nil, fmt.Errorf("db: unsupported database.type %q", sec.Type)
	}
}

func openSQLite(ctx context.Context, sec config.DatabaseSection, log zerolog.Logger) (*Handle, error) {
	metaDSN := ResolveMetaDSN(sec)
	meta, err := sqlitemeta.Open(ctx, metaDSN, log)
	if err != nil {
		return nil, err
	}
	if err := migrateLegacyMetaSQLite(ctx, sec.DSN, meta); err != nil {
		_ = meta.Close()
		return nil, fmt.Errorf("db: legacy meta migration: %w", err)
	}
	ev, err := sqlite.Open(ctx, sec.DSN, nil, log)
	if err != nil {
		_ = meta.Close()
		return nil, err
	}
	store := newCompositeStore(ev, meta, ev, meta)
	return &Handle{
		Store:         store,
		EventNotifier: storage.NoopNotifier{},
		closeFn: func() error {
			err1 := meta.Close()
			err2 := ev.Close()
			if err1 != nil {
				return err1
			}
			return err2
		},
	}, nil
}
