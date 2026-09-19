package sqlevent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/uptrace/bun"
)

// Store is a SQLite-compatible event store with a single-writer queue and concurrent reads.
type Store struct {
	wq       *sqlitewriter.Queue
	notifier storage.EventNotifier
	dbPath   string
	engine   string
}

var _ storage.EventStore = (*Store)(nil)

func (s *Store) db() *bun.DB {
	return s.wq.DB()
}

// DB exposes the read connection (for tests).
func (s *Store) DB() *bun.DB {
	return s.db()
}

func (s *Store) runWrite(ctx context.Context, label string, run func(ctx context.Context, db bun.IDB) error) error {
	return s.wq.RunWrite(ctx, label, run)
}

// Close stops the writer and closes the database.
func (s *Store) Close() error {
	_ = s.notifier.Close()
	return s.wq.Close()
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
		return fmt.Errorf("%s: ephemeral events are not stored", s.engine)
	}
	err := s.runWrite(ctx, "SaveEvent", func(ctx context.Context, db bun.IDB) error {
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
	err := s.db().NewSelect().Model(&tagRows).
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
	if err := s.db().NewSelect().Model(&tagRows).
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
	q := s.db().NewSelect().Model(&rows)
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

// QueryEventSyncItems returns id+created_at rows matching filter, ascending for NIP-77.
func (s *Store) QueryEventSyncItems(ctx context.Context, filter nostr.Filter) ([]storage.SyncItem, error) {
	if filter.HasSearch() {
		return nil, nil
	}
	type row struct {
		ID        string `bun:"id"`
		CreatedAt int64  `bun:"created_at"`
	}
	var rows []row
	q := s.db().NewSelect().Model((*storage.EventRow)(nil)).Column("id", "created_at")
	q = applyFilterQuery(q, &filter)
	q = q.Order("created_at ASC", "id ASC")
	if err := q.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]storage.SyncItem, len(rows))
	for i, r := range rows {
		out[i] = storage.SyncItem{ID: r.ID, CreatedAt: r.CreatedAt}
	}
	return out, nil
}

// DeleteEvent removes an event and its tags.
func (s *Store) DeleteEvent(ctx context.Context, id string) error {
	return s.runWrite(ctx, "DeleteEvent", func(ctx context.Context, db bun.IDB) error {
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
		err := s.db().QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count)
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
	err := s.db().QueryRowContext(ctx, fullSQL, allArgs...).Scan(&count)
	return count, err
}

// HasEventID implements storage.Store.
func (s *Store) HasEventID(ctx context.Context, id string) (bool, error) {
	n, err := s.db().NewSelect().Model((*storage.EventRow)(nil)).Where("id = ?", id).Limit(1).Count(ctx)
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

	rows, err := s.db().QueryContext(ctx, sb.String(), args...)
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
	q := s.db().NewSelect().Model((*storage.EventRow)(nil)).
		Where("id LIKE ?", p+"%")
	if requireSameH && groupID != "" {
		q = q.Where("id IN (SELECT event_id FROM event_tags WHERE name = 'h' AND value = ?)", groupID)
	}
	return q.Exists(ctx)
}

// GetLatestGroupMetadata39000 implements storage.Store (NIP-29).
func (s *Store) GetLatestGroupMetadata39000(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error) {
	var row storage.EventRow
	err := s.db().NewSelect().Model(&row).
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
	err := s.db().NewSelect().Model(&row).
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
	err := s.db().NewSelect().Model(&row).
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

