package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/michmich112/congee/internal/nostr"
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
	dbPath    string // main database file path (for AdminStorageSnapshot size)
}

var _ storage.EventStore = (*Store)(nil)

// Open opens a SQLite database (WAL, Bun + sqliteshim), runs migrations, and starts the writer loop.
// log is used for optional debug traces (use zerolog.Nop() when silent).
func Open(ctx context.Context, dsn string, notifier storage.EventNotifier, log zerolog.Logger) (*Store, error) {
	log = log.With().Str("engine", "sqlite").Logger()
	if !sqliteshim.HasDriver() {
		return nil, errors.New("sqlite: sqliteshim driver not available for this build target")
	}
	if notifier == nil {
		notifier = storage.NoopNotifier{}
	}

	log.Debug().Int("dsn_len", len(strings.TrimSpace(dsn))).Msg("open: sql.Open")
	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqldb.SetMaxOpenConns(64)
	sqldb.SetMaxIdleConns(64)

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		log.Warn().Err(err).Msg("open: initial ping failed")
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	log.Debug().Msg("open: ping ok")

	log.Debug().Msg("open: pragma foreign_keys")
	if _, err := sqldb.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = sqldb.Close()
		log.Warn().Err(err).Msg("open: foreign_keys pragma failed")
		return nil, fmt.Errorf("sqlite: foreign_keys: %w", err)
	}
	log.Debug().Msg("open: pragma journal_mode=WAL")
	if _, err := sqldb.ExecContext(ctx, `PRAGMA journal_mode = WAL;`); err != nil {
		_ = sqldb.Close()
		log.Warn().Err(err).Msg("open: journal_mode pragma failed")
		return nil, fmt.Errorf("sqlite: journal_mode: %w", err)
	}
	log.Debug().Msg("open: pragma busy_timeout")
	if _, err := sqldb.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		_ = sqldb.Close()
		log.Warn().Err(err).Msg("open: busy_timeout pragma failed")
		return nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	log.Debug().Msg("open: running schema migrations")
	if err := runMigrations(ctx, db, log); err != nil {
		_ = db.Close()
		log.Warn().Err(err).Msg("open: schema migrations failed")
		return nil, err
	}
	log.Debug().Msg("open: schema migrations done")

	baseCtx, cancel := context.WithCancel(context.Background())
	dbPath, err := sqliteMainFilePath(dsn)
	if err != nil {
		cancel()
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: resolve db path: %w", err)
	}
	s := &Store{
		db:        db,
		notifier:  notifier,
		writes:    make(chan writeTask, 256),
		cancel:    cancel,
		baseCtx:   baseCtx,
		closedErr: errors.New("sqlite: store closed"),
		dbPath:    dbPath,
	}
	s.wg.Add(1)
	go s.writerLoop(baseCtx)
	log.Debug().Msg("open: writer loop started; store ready")
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
	// Always wait for the writer to finish once the task is queued. The closure may
	// capture caller-stack values that the writer mutates during the query (e.g. bun
	// RETURNING scans into a row struct). Returning early on ctx.Done() would race
	// with those writes.
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
	_ = s.notifier.Close()
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
			tags := make([]storage.EventTagRow, 0, len(ev.Tags))
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
				tags = append(tags, storage.EventTagRow{
					EventID:  ev.ID,
					Pos:      i,
					Name:     name,
					Value:    val,
					FullJSON: string(full),
				})
			}
			if len(tags) > 0 {
				if _, err := tx.NewInsert().Model(&tags).Exec(ctx); err != nil {
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
	tags, err := storage.GroupTagRows(tagRows)
	if err != nil {
		return nil, err
	}
	return rowToEventWithTags(row, tags[row.ID]), nil
}

func (s *Store) tagsByEventID(ctx context.Context, ids []string) (map[string][][]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var tagRows []storage.EventTagRow
	if err := s.db.NewSelect().Model(&tagRows).
		Where("event_id IN (?)", bun.In(ids)).
		Order("event_id ASC", "pos ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return storage.GroupTagRows(tagRows)
}

func rowToEventWithTags(row *storage.EventRow, tags [][]string) *nostr.Event {
	if tags == nil {
		tags = [][]string{}
	}
	ev := &nostr.Event{
		ID:        row.ID,
		PubKey:    row.Pubkey,
		CreatedAt: row.CreatedAt,
		Kind:      row.Kind,
		Tags:      tags,
		Content:   row.Content,
		Sig:       row.Sig,
	}
	return ev
}

func applyFilterQuery(q *bun.SelectQuery, f *nostr.Filter) *bun.SelectQuery {
	return applyFilterQueryPrefix(q, f, "")
}

// applyFilterQueryPrefix adds structural filter clauses. If prefix is non-empty (e.g. "events."),
// columns are qualified for JOIN queries.
func applyFilterQueryPrefix(q *bun.SelectQuery, f *nostr.Filter, prefix string) *bun.SelectQuery {
	col := func(name string) string {
		if prefix == "" {
			return name
		}
		return prefix + name
	}
	if len(f.IDs) > 0 {
		q = q.Where(col("id")+" IN (?)", bun.In(f.IDs))
	}
	if len(f.Authors) > 0 {
		q = q.Where(col("pubkey")+" IN (?)", bun.In(f.Authors))
	}
	if len(f.Kinds) > 0 {
		q = q.Where(col("kind")+" IN (?)", bun.In(f.Kinds))
	}
	if f.Since != nil {
		q = q.Where(col("created_at")+" >= ?", *f.Since)
	}
	if f.Until != nil {
		q = q.Where(col("created_at")+" <= ?", *f.Until)
	}
	for key, vals := range f.Tag {
		if len(vals) == 0 {
			q = q.Where("FALSE")
			return q
		}
		name := key[1:]
		q = q.Where(col("id")+" IN (SELECT event_id FROM event_tags WHERE name = ? AND value IN (?))",
			name, bun.In(vals))
	}
	return q
}

func (s *Store) selectRows(ctx context.Context, f *nostr.Filter, applyLimits bool) ([]storage.EventRow, error) {
	if f != nil && f.HasSearch() {
		return nil, nil
	}
	var rows []storage.EventRow
	q := s.db.NewSelect().Model(&rows)
	q = applyFilterQuery(q, f)
	q = q.Order("created_at DESC", "id ASC")
	if lim := storage.FilterSQLLimit(f, applyLimits); lim != nil {
		q = q.Limit(*lim)
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
	tagsMap, err := s.tagsByEventID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*nostr.Event, 0, len(ids))
	for _, id := range ids {
		row := byID[id]
		out = append(out, rowToEventWithTags(&row, tagsMap[row.ID]))
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

// CountEvents returns how many distinct events match any filter (OR) via SQL COUNT.
func (s *Store) CountEvents(ctx context.Context, filters []nostr.Filter) (int, error) {
	if filters == nil || len(filters) == 0 {
		var count int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count)
		return count, err
	}

	subQueries := make([]string, 0, len(filters))
	var allArgs []interface{}
	for i := range filters {
		q, args, skip := storage.CountFilterSubQuery(&filters[i])
		if skip {
			continue
		}
		subQueries = append(subQueries, q)
		allArgs = append(allArgs, args...)
	}
	if len(subQueries) == 0 {
		return 0, nil
	}

	var fullSQL string
	if len(subQueries) == 1 {
		fullSQL = "SELECT COUNT(*) FROM (" + subQueries[0] + ") t"
	} else {
		fullSQL = "SELECT COUNT(*) FROM (" + strings.Join(subQueries, " UNION ") + ") t"
	}

	var count int
	err := s.db.QueryRowContext(ctx, fullSQL, allArgs...).Scan(&count)
	return count, err
}

// HasEventID implements storage.Store.
func (s *Store) HasEventID(ctx context.Context, id string) (bool, error) {
	n, err := s.db.NewSelect().Model((*storage.EventRow)(nil)).Where("id = ?", id).Limit(1).Count(ctx)
	return n > 0, err
}

// SearchEvents runs FTS5 on mirrored content (NIP-50), ordered by bm25 rank (lower is better).
func (s *Store) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	q := strings.TrimSpace(searchQuery)
	if q == "" {
		return nil, nil
	}
	cons := constraints.WithoutSearch()
	matchExpr := fts5Phrase(q)
	if matchExpr == "" {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString(`SELECT events.id, events.pubkey, events.created_at, events.kind, events.content, events.sig, events.d_tag
FROM events
INNER JOIN event_fts ON event_fts.event_id = events.id
WHERE event_fts MATCH ?`)
	args := []interface{}{matchExpr}
	sqliteAppendSearchFilter(&sb, &args, &cons)
	sb.WriteString(` ORDER BY bm25(event_fts) ASC`)
	if lim := storage.FilterSQLLimit(&cons, true); lim != nil {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", *lim))
	}

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	eventRows := make([]storage.EventRow, 0, 64)
	for rows.Next() {
		var row storage.EventRow
		if err := rows.Scan(&row.ID, &row.Pubkey, &row.CreatedAt, &row.Kind, &row.Content, &row.Sig, &row.DTag); err != nil {
			return nil, err
		}
		eventRows = append(eventRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]string, len(eventRows))
	for i, r := range eventRows {
		ids[i] = r.ID
	}
	tagsMap, err := s.tagsByEventID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*nostr.Event, 0, len(eventRows))
	for i := range eventRows {
		out = append(out, rowToEventWithTags(&eventRows[i], tagsMap[eventRows[i].ID]))
	}
	return out, nil
}

func sqliteAppendSearchFilter(sb *strings.Builder, args *[]interface{}, f *nostr.Filter) {
	if len(f.IDs) > 0 {
		sb.WriteString(" AND events.id IN (")
		for i, id := range f.IDs {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			*args = append(*args, id)
		}
		sb.WriteString(")")
	}
	if len(f.Authors) > 0 {
		sb.WriteString(" AND events.pubkey IN (")
		for i, a := range f.Authors {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			*args = append(*args, a)
		}
		sb.WriteString(")")
	}
	if len(f.Kinds) > 0 {
		sb.WriteString(" AND events.kind IN (")
		for i, k := range f.Kinds {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			*args = append(*args, k)
		}
		sb.WriteString(")")
	}
	if f.Since != nil {
		sb.WriteString(" AND events.created_at >= ?")
		*args = append(*args, *f.Since)
	}
	if f.Until != nil {
		sb.WriteString(" AND events.created_at <= ?")
		*args = append(*args, *f.Until)
	}
	for key, vals := range f.Tag {
		if len(vals) == 0 {
			sb.WriteString(" AND FALSE")
			continue
		}
		name := key[1:]
		sb.WriteString(" AND events.id IN (SELECT event_id FROM event_tags WHERE name = ? AND value IN (")
		*args = append(*args, name)
		for i, v := range vals {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			*args = append(*args, v)
		}
		sb.WriteString("))")
	}
}

// fts5Phrase wraps the user string as a single FTS5 phrase (quotes escaped per SQLite rules).
func fts5Phrase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func normalizeIDPrefix8(prefix string) string {
	p := strings.TrimSpace(strings.ToLower(prefix))
	if len(p) > 8 {
		p = p[:8]
	}
	return p
}

// EventIDPrefixExists implements storage.Store (NIP-29).
func (s *Store) EventIDPrefixExists(ctx context.Context, prefix string, groupID string, requireSameH bool) (bool, error) {
	p := normalizeIDPrefix8(prefix)
	if p == "" {
		return false, nil
	}
	q := s.db.NewSelect().Model((*storage.EventRow)(nil)).
		Where("id LIKE ?", p+"%")
	if requireSameH && groupID != "" {
		q = q.Where("id IN (SELECT event_id FROM event_tags WHERE name = 'h' AND value = ?)", groupID)
	}
	return q.Exists(ctx)
}

// GetLatestGroupMetadata39000 implements storage.Store (NIP-29).
func (s *Store) GetLatestGroupMetadata39000(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	var row storage.EventRow
	err := s.db.NewSelect().Model(&row).
		Where("pubkey = ? AND kind = ? AND d_tag = ?", relayPubkey, 39000, groupID).
		Order("created_at DESC", "id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.rowToEvent(ctx, &row)
}

// GetLatestGroupAdmins39001 implements storage.Store (NIP-29).
func (s *Store) GetLatestGroupAdmins39001(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	var row storage.EventRow
	err := s.db.NewSelect().Model(&row).
		Where("pubkey = ? AND kind = ? AND d_tag = ?", relayPubkey, nostr.NIP29KindGroupAdmins, groupID).
		Order("created_at DESC", "id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.rowToEvent(ctx, &row)
}

// IsGroupMember implements storage.Store (NIP-29).
func (s *Store) IsGroupMember(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error) {
	var row storage.EventRow
	err := s.db.NewSelect().Model(&row).
		TableExpr("events e").
		Column("e.id", "e.pubkey", "e.created_at", "e.kind", "e.content", "e.sig", "e.d_tag").
		Join("INNER JOIN event_tags et_h ON et_h.event_id = e.id AND et_h.name = 'h' AND et_h.value = ?", groupID).
		Join("INNER JOIN event_tags et_p ON et_p.event_id = e.id AND et_p.name = 'p' AND et_p.value = ?", memberPubkey).
		Where("e.pubkey = ?", relayPubkey).
		Where("e.kind IN (?, ?)", 9000, 9001).
		Order("e.created_at DESC", "e.id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	switch row.Kind {
	case 9000:
		return true, nil
	case 9001:
		return false, nil
	default:
		return false, nil
	}
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

// HasAuditDuplicate implements storage.Store.
func (s *Store) HasAuditDuplicate(ctx context.Context, e storage.AuditEntry) (bool, error) {
	n, err := s.db.NewSelect().Model((*storage.AuditLogRow)(nil)).
		Where("created_at = ? AND action = ? AND detail = ? AND pubkey = ?",
			e.CreatedAt, e.Action, e.Detail, e.Pubkey).
		Limit(1).
		Count(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	evID := storage.ExtractAuditDetailEventID(e.Detail)
	if evID == "" {
		return false, nil
	}
	pat := "%event_id=" + evID + "%"
	n2, err := s.db.NewSelect().Model((*storage.AuditLogRow)(nil)).
		Where("pubkey = ? AND LOWER(detail) LIKE ?", e.Pubkey, pat).
		Limit(1).
		Count(ctx)
	return n2 > 0, err
}

func applySQLiteAuditLogFilters(q *bun.SelectQuery, query storage.AuditQuery) *bun.SelectQuery {
	if query.Since > 0 {
		q = q.Where("created_at >= ?", query.Since)
	}
	if query.Until > 0 {
		q = q.Where("created_at <= ?", query.Until)
	}
	if query.Action != "" {
		q = q.Where("action = ?", query.Action)
	}
	if query.Pubkey != "" {
		q = q.Where("pubkey = ?", query.Pubkey)
	}
	if sql, args := storage.AuditDetailKindSuffixMatchOr(true, query.Kinds); sql != "" {
		q = q.Where(sql, args...)
	}
	return q
}

func (s *Store) QueryAuditLog(ctx context.Context, query storage.AuditQuery) ([]storage.AuditEntry, error) {
	var rows []storage.AuditLogRow
	q := applySQLiteAuditLogFilters(s.db.NewSelect().Model(&rows), query).Order("created_at DESC")
	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}
	if query.Offset > 0 {
		q = q.Offset(query.Offset)
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

func (s *Store) CountAuditLog(ctx context.Context, query storage.AuditQuery) (int64, error) {
	q := applySQLiteAuditLogFilters(s.db.NewSelect().Model((*storage.AuditLogRow)(nil)), query)
	n, err := q.Count(ctx)
	return int64(n), err
}

func (s *Store) ListDistinctAuditKinds(ctx context.Context, scanLimit int) ([]int, error) {
	if scanLimit <= 0 {
		scanLimit = storage.DefaultAuditKindsScanLimit
	}
	if scanLimit > storage.MaxAuditKindsScanLimit {
		scanLimit = storage.MaxAuditKindsScanLimit
	}
	var rows []struct {
		Detail string `bun:"detail"`
	}
	err := s.db.NewSelect().
		Model((*storage.AuditLogRow)(nil)).
		Column("detail").
		Order("created_at DESC").
		Limit(scanLimit).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]struct{})
	for _, r := range rows {
		if k, ok := storage.ParseAuditDetailTrailingKind(r.Detail); ok {
			seen[k] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Ints(out)
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

func (s *Store) SaveWSConnectionSession(ctx context.Context, e storage.WSConnectionSession) (int64, error) {
	var insertedID int64
	err := s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		row := storage.WSConnectionSessionToRow(e)
		_, err := db.NewInsert().Model(&row).Returning("id").Exec(ctx)
		if err != nil {
			return err
		}
		insertedID = row.ID
		return nil
	})
	return insertedID, err
}

// QueryWSConnectionSessions implements storage.Store (newest ended first).
func (s *Store) QueryWSConnectionSessions(ctx context.Context, q storage.WSConnectionSessionQuery) ([]storage.WSConnectionSession, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	var rows []storage.WSConnectionSessionRow
	err := s.db.NewSelect().Model(&rows).
		Order("ended_unix DESC", "id DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.WSConnectionSession, len(rows))
	for i := range rows {
		out[i] = storage.WSConnectionSessionFromRow(rows[i])
	}
	return out, nil
}

// CountWSConnectionSessions implements storage.Store.
func (s *Store) CountWSConnectionSessions(ctx context.Context) (int64, error) {
	n, err := s.db.NewSelect().Model((*storage.WSConnectionSessionRow)(nil)).Count(ctx)
	return int64(n), err
}

// GetWSConnectionSessionByID implements storage.Store.
func (s *Store) GetWSConnectionSessionByID(ctx context.Context, id int64) (*storage.WSConnectionSession, error) {
	var row storage.WSConnectionSessionRow
	err := s.db.NewSelect().Model(&row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	e := storage.WSConnectionSessionFromRow(row)
	return &e, nil
}

// PurgeWSConnectionSessionsBefore implements storage.Store.
func (s *Store) PurgeWSConnectionSessionsBefore(ctx context.Context, olderThanUnix int64) (int64, error) {
	var n int64
	err := s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
		res, err := db.NewDelete().Model((*storage.WSConnectionSessionRow)(nil)).
			Where("ended_unix < ?", olderThanUnix).
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
