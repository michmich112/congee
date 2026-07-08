package sqlevent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

// HandlesOpener opens a database and returns sql.DB + bun.DB handles.
type HandlesOpener func(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error)

// OpenConfig configures opening a SQLite-compatible event store (sqlite or turso/libsql).
type OpenConfig struct {
	Engine        string // "sqlite" or "turso"
	DSN           string
	Notifier      storage.EventNotifier
	Log           zerolog.Logger
	OpenHandles   HandlesOpener
	ResolveDBPath func(dsn string) (string, error)
}

// Open opens a SQLite-compatible event store, runs migrations, and starts the writer loop.
func Open(ctx context.Context, cfg OpenConfig) (*Store, error) {
	if cfg.OpenHandles == nil {
		return nil, errors.New("sqlevent: OpenHandles is required")
	}
	if cfg.ResolveDBPath == nil {
		return nil, errors.New("sqlevent: ResolveDBPath is required")
	}
	engine := strings.TrimSpace(cfg.Engine)
	if engine == "" {
		engine = "sqlite"
	}
	log := cfg.Log.With().Str("engine", engine).Logger()
	if cfg.Notifier == nil {
		cfg.Notifier = storage.NoopNotifier{}
	}

	normDSN := sqlitewriter.NormalizeDSN(cfg.DSN)
	log.Debug().Int("dsn_len", len(strings.TrimSpace(cfg.DSN))).Msg("open: sql.Open")
	sqldb, db, err := cfg.OpenHandles(ctx, normDSN, log)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", engine, err)
	}
	log.Debug().Msg("open: ping and pragmas ok")

	log.Debug().Msg("open: running schema migrations")
	if err := RunMigrations(ctx, db, engine, log); err != nil {
		_ = db.Close()
		log.Warn().Err(err).Msg("open: schema migrations failed")
		return nil, err
	}
	log.Debug().Msg("open: schema migrations done")

	dbPath, err := cfg.ResolveDBPath(cfg.DSN)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: resolve db path: %w", engine, err)
	}
	wq := sqlitewriter.New(sqldb, db, sqlitewriter.Options{
		Engine: engine,
		Log:    log,
		DSN:    normDSN,
	})
	s := &Store{
		wq:       wq,
		notifier: cfg.Notifier,
		dbPath:   dbPath,
		engine:   engine,
	}
	log.Debug().Msg("open: writer queue started; store ready")
	return s, nil
}
