package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"

	"github.com/uptrace/bun/driver/sqliteshim"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)

const defaultQueryLimit = 500

type writeTask struct {
	run  func(ctx context.Context, db bun.IDB) error
	done chan<- error
}

// Store is a SQLite-backed storage.Store with a single-writer queue and concurrent reads.
type Store struct {
	db        *bun.DB
	notifier  storage.EventNotifier
	writes    chan writeTask
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	baseCtx   context.Context
	shutdown  atomic.Bool
	closedErr error
}

var _ storage.Store = (*Store)(nil)

// Open opens a SQLite database (WAL, Bun + sqliteshim), runs migrations, and starts the writer loop.
func Open(ctx context.Context, dsn string, notifier storage.EventNotifier) (*Store, error) {
	if !sqliteshim.HasDriver() {
		return nil, errors.New("sqlite: sqliteshim driver not available for this build target")
	}
	if notifier == nil {
		notifier = storage.NoopNotifier{}
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqldb.SetMaxOpenConns(64)
	sqldb.SetMaxIdleConns(64)

	if _, err := sqldb.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlite: foreign_keys: %w", err)
	}
	if _, err := sqldb.ExecContext(ctx, `PRAGMA journal_mode = WAL;`); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlite: journal_mode: %w", err)
	}
	if _, err := sqldb.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	baseCtx, cancel := context.WithCancel(context.Background())
	s := &Store{
		db:        db,
		notifier:  notifier,
		writes:    make(chan writeTask, 256),
		cancel:    cancel,
		baseCtx:   baseCtx,
		closedErr: errors.New("sqlite: store closed"),
	}
	s.wg.Add(1)
	go s.writerLoop(baseCtx)
	return s, nil
}

func normalizeDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "file:congee.db?cache=shared"
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
	case <-ctx.Done():
		return ctx.Err()
	case <-s.baseCtx.Done():
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

func extractDTag(tags [][]string) string {
	for _, t := range tags {
		if len(t) > 0 && t[0] == "d" {
			if len(t) > 1 {
				return t[1]
			}
			return ""
		}
	}
	return ""
}

// SaveEvent persists an event, replacing prior replaceable/addressable rows per NIP-01.
func (s *Store) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	if nostr.IsEphemeral(ev.Kind) {
		return errors.New("sqlite: ephemeral events are not stored")
	}
	err := s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			switch nostr.ClassifyKind(ev.Kind) {
			case nostr.KindReplaceable:
				if _, err := tx.NewDelete().Model((*storage.EventRow)(nil)).
					Where("pubkey = ? AND kind = ?", ev.PubKey, ev.Kind).
					Exec(ctx); err != nil {
					return err
				}
			case nostr.KindAddressable:
				dt := extractDTag(ev.Tags)
				if _, err := tx.NewDelete().Model((*storage.EventRow)(nil)).
					Where("pubkey = ? AND kind = ? AND d_tag = ?", ev.PubKey, ev.Kind, dt).
					Exec(ctx); err != nil {
					return err
				}
			}

			row := storage.EventRow{
				ID:        ev.ID,
				Pubkey:    ev.PubKey,
				CreatedAt: ev.CreatedAt,
				Kind:      ev.Kind,
				Content:   ev.Content,
				Sig:       ev.Sig,
				DTag:      extractDTag(ev.Tags),
			}
			if _, err := tx.NewInsert().Model(&row).Exec(ctx); err != nil {
				return err
			}
			for i, t := range ev.Tags {
				full, err := json.Marshal(t)
				if err != nil {
					return err
				}
				val := ""
				if len(t) > 1 {
					val = t[1]
				}
				name := ""
				if len(t) > 0 {
					name = t[0]
				}
				tag := storage.EventTagRow{
					EventID:  ev.ID,
					Pos:      i,
					Name:     name,
					Value:    val,
					FullJSON: string(full),
				}
				if _, err := tx.NewInsert().Model(&tag).Exec(ctx); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	s.notifier.Notify(ev.ID)
	return nil
}

func (s *Store) rowToEvent(ctx context.Context, row *storage.EventRow) (*nostr.Event, error) {
	var tagRows []storage.EventTagRow
	err := s.db.NewSelect().Model(&tagRows).
		Where("event_id = ?", row.ID).
		Order("pos ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	tags := make([][]string, 0, len(tagRows))
	for _, tr := range tagRows {
		var parts []string
		if err := json.Unmarshal([]byte(tr.FullJSON), &parts); err != nil {
			return nil, err
		}
		tags = append(tags, parts)
	}
	return &nostr.Event{
		ID:        row.ID,
		PubKey:    row.Pubkey,
		CreatedAt: row.CreatedAt,
		Kind:      row.Kind,
		Tags:      tags,
		Content:   row.Content,
		Sig:       row.Sig,
	}, nil
}

func filterLimit(f *nostr.Filter, applyLimits bool) int {
	if !applyLimits {
		return math.MaxInt32
	}
	if f.Limit != nil && *f.Limit > 0 {
		return *f.Limit
	}
	return defaultQueryLimit
}

func applyFilterQuery(q *bun.SelectQuery, f *nostr.Filter) *bun.SelectQuery {
	if len(f.IDs) > 0 {
		q = q.Where("id IN (?)", bun.In(f.IDs))
	}
	if len(f.Authors) > 0 {
		q = q.Where("pubkey IN (?)", bun.In(f.Authors))
	}
	if len(f.Kinds) > 0 {
		q = q.Where("kind IN (?)", bun.In(f.Kinds))
	}
	if f.Since != nil {
		q = q.Where("created_at >= ?", *f.Since)
	}
	if f.Until != nil {
		q = q.Where("created_at <= ?", *f.Until)
	}
	for key, vals := range f.Tag {
		if len(vals) == 0 {
			q = q.Where("FALSE")
			return q
		}
		name := key[1:]
		q = q.Where("id IN (SELECT event_id FROM event_tags WHERE name = ? AND value IN (?))",
			name, bun.In(vals))
	}
	return q
}

func (s *Store) selectRows(ctx context.Context, f *nostr.Filter, applyLimits bool) ([]storage.EventRow, error) {
	var rows []storage.EventRow
	q := s.db.NewSelect().Model(&rows)
	q = applyFilterQuery(q, f)
	q = q.Order("created_at DESC", "id ASC")
	lim := filterLimit(f, applyLimits)
	if lim < math.MaxInt32 {
		q = q.Limit(lim)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

// QueryEvents returns events matching any of the filters (OR), newest first.
func (s *Store) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	byID := make(map[string]storage.EventRow)
	for i := range filters {
		rows, err := s.selectRows(ctx, &filters[i], true)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			byID[r.ID] = r
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := byID[ids[i]], byID[ids[j]]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		return a.ID < b.ID
	})
	out := make([]*nostr.Event, 0, len(ids))
	for _, id := range ids {
		row := byID[id]
		ev, err := s.rowToEvent(ctx, &row)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

// DeleteEvent removes an event and its tags.
func (s *Store) DeleteEvent(ctx context.Context, id string) error {
	return s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if _, err := tx.NewDelete().Model((*storage.EventTagRow)(nil)).Where("event_id = ?", id).Exec(ctx); err != nil {
				return err
			}
			_, err := tx.NewDelete().Model((*storage.EventRow)(nil)).Where("id = ?", id).Exec(ctx)
			return err
		})
	})
}

// CountEvents returns how many distinct events match any filter (OR). Loads matching IDs in memory.
func (s *Store) CountEvents(ctx context.Context, filters []nostr.Filter) (int, error) {
	if len(filters) == 0 {
		return 0, nil
	}
	byID := make(map[string]struct{})
	for i := range filters {
		rows, err := s.selectRows(ctx, &filters[i], false)
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			byID[r.ID] = struct{}{}
		}
	}
	return len(byID), nil
}

// SearchEvents is not implemented (NIP-50).
func (s *Store) SearchEvents(ctx context.Context, query string, limit int) ([]*nostr.Event, error) {
	_ = ctx
	_ = query
	_ = limit
	return nil, storage.ErrSearchNotImplemented
}

func (s *Store) SaveAuditEntry(ctx context.Context, e storage.AuditEntry) error {
	return s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		row := storage.AuditLogRow{
			CreatedAt: e.CreatedAt,
			Action:    e.Action,
			Detail:    e.Detail,
			Pubkey:    e.Pubkey,
		}
		_, err := db.NewInsert().Model(&row).Exec(ctx)
		return err
	})
}

func (s *Store) QueryAuditLog(ctx context.Context, since, until int64, limit int) ([]storage.AuditEntry, error) {
	var rows []storage.AuditLogRow
	q := s.db.NewSelect().Model(&rows).Order("created_at DESC")
	if since > 0 {
		q = q.Where("created_at >= ?", since)
	}
	if until > 0 {
		q = q.Where("created_at <= ?", until)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]storage.AuditEntry, len(rows))
	for i, r := range rows {
		out[i] = storage.AuditEntry{
			CreatedAt: r.CreatedAt,
			Action:    r.Action,
			Detail:    r.Detail,
			Pubkey:    r.Pubkey,
		}
	}
	return out, nil
}

func (s *Store) PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error) {
	var n int64
	err := s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		res, err := db.NewDelete().Model((*storage.AuditLogRow)(nil)).
			Where("created_at < ?", olderThanUnix).
			Exec(ctx)
		if err != nil {
			return err
		}
		v, err := res.RowsAffected()
		n = v
		return err
	})
	return n, err
}

func (s *Store) SaveConfigChange(ctx context.Context, c storage.ConfigChange) error {
	return s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		row := storage.ConfigChangelogRow{
			CreatedAt: c.CreatedAt,
			Summary:   c.Summary,
			JSONDiff:  c.JSONDiff,
		}
		_, err := db.NewInsert().Model(&row).Exec(ctx)
		return err
	})
}

func (s *Store) QueryConfigChangelog(ctx context.Context, limit int) ([]storage.ConfigChange, error) {
	var rows []storage.ConfigChangelogRow
	q := s.db.NewSelect().Model(&rows).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]storage.ConfigChange, len(rows))
	for i, r := range rows {
		out[i] = storage.ConfigChange{
			CreatedAt: r.CreatedAt,
			Summary:   r.Summary,
			JSONDiff:  r.JSONDiff,
		}
	}
	return out, nil
}
