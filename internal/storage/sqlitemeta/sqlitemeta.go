package sqlitemeta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"

	"github.com/uptrace/bun/driver/sqliteshim"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)

type writeTask struct {
	run  func(ctx context.Context, db bun.IDB) error
	done chan<- error
}

const writerQueueCapacity = 1024

// Store is a SQLite-backed storage.MetaStore with a single-writer queue and concurrent reads.
type Store struct {
	db        *bun.DB
	writes    chan writeTask
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	baseCtx   context.Context
	shutdown  atomic.Bool
	closedErr error
	dbPath    string
}

var _ storage.MetaStore = (*Store)(nil)

// Open opens the meta SQLite database (WAL, Bun + sqliteshim), runs migrations, and starts the writer loop.
func Open(ctx context.Context, dsn string, log zerolog.Logger) (*Store, error) {
	log = log.With().Str("engine", "sqlitemeta").Logger()
	if !sqliteshim.HasDriver() {
		return nil, errors.New("sqlitemeta: sqliteshim driver not available for this build target")
	}

	log.Debug().Int("dsn_len", len(strings.TrimSpace(dsn))).Msg("open: sql.Open")
	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("sqlitemeta: open: %w", err)
	}
	sqldb.SetMaxOpenConns(64)
	sqldb.SetMaxIdleConns(64)

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlitemeta: ping: %w", err)
	}

	if _, err := sqldb.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlitemeta: foreign_keys: %w", err)
	}
	if _, err := sqldb.ExecContext(ctx, `PRAGMA journal_mode = WAL;`); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlitemeta: journal_mode: %w", err)
	}
	if _, err := sqldb.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlitemeta: busy_timeout: %w", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	if err := runMigrations(ctx, db, log); err != nil {
		_ = db.Close()
		return nil, err
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	dbPath, err := mainFilePath(dsn)
	if err != nil {
		cancel()
		_ = db.Close()
		return nil, fmt.Errorf("sqlitemeta: resolve db path: %w", err)
	}
	s := &Store{
		db:        db,
		writes:    make(chan writeTask, writerQueueCapacity),
		cancel:    cancel,
		baseCtx:   baseCtx,
		closedErr: errors.New("sqlitemeta: store closed"),
		dbPath:    dbPath,
	}
	s.wg.Add(1)
	go s.writerLoop(baseCtx)
	return s, nil
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

func (s *Store) writerLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case task := <-s.writes:
					task.done <- s.closedErr
				default:
					return
				}
			}
		case task := <-s.writes:
			runCtx := context.Background()
			err := task.run(runCtx, s.db)
			task.done <- err
		}
	}
}

func (s *Store) runWrite(ctx context.Context, run func(ctx context.Context, db bun.IDB) error) error {
	if s.shutdown.Load() {
		return s.closedErr
	}
	done := make(chan error, 1)
	task := writeTask{run: run, done: done}
	select {
	case s.writes <- task:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.baseCtx.Done():
		return s.closedErr
	}
	select {
	case err := <-done:
		return err
	case <-s.baseCtx.Done():
		err := <-done
		_ = err
		return s.closedErr
	}
}

// Close stops the writer and closes the database.
func (s *Store) Close() error {
	s.shutdown.Store(true)
	s.cancel()
	s.wg.Wait()
	return s.db.Close()
}
