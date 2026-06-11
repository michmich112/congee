package sqlitemeta

import (
	"context"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

// Store is a SQLite-backed storage.MetaStore with a single-writer queue and concurrent reads.
type Store struct {
	wq     *sqlitewriter.Queue
	dbPath string
}

var _ storage.MetaStore = (*Store)(nil)

// Open opens the meta SQLite database (WAL, Bun + sqliteshim), runs migrations, and starts the writer loop.
func Open(ctx context.Context, dsn string, log zerolog.Logger) (*Store, error) {
	log = log.With().Str("engine", "sqlitemeta").Logger()

	normDSN := normalizeDSN(dsn)

	log.Debug().Int("dsn_len", len(strings.TrimSpace(dsn))).Msg("open: sql.Open")
	sqldb, db, err := sqlitewriter.OpenHandles(ctx, normDSN, log)
	if err != nil {
		return nil, fmt.Errorf("sqlitemeta: %w", err)
	}

	if err := runMigrations(ctx, db, log); err != nil {
		_ = db.Close()
		return nil, err
	}

	dbPath, err := mainFilePath(dsn)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlitemeta: resolve db path: %w", err)
	}
	wq := sqlitewriter.New(sqldb, db, sqlitewriter.Options{
		Engine: "sqlitemeta",
		Log:    log,
		DSN:    normDSN,
	})
	return &Store{wq: wq, dbPath: dbPath}, nil
}

func (s *Store) db() *bun.DB {
	return s.wq.DB()
}

func (s *Store) runWrite(ctx context.Context, label string, run func(ctx context.Context, db bun.IDB) error) error {
	return s.wq.RunWrite(ctx, label, run)
}

// Close stops the writer and closes the database.
func (s *Store) Close() error {
	return s.wq.Close()
}

func normalizeDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "file:congee-meta.db?cache=shared"
	}
	if strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	return "file:" + dsn + "?cache=shared"
}
