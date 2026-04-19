package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

const defaultQueryLimit = 500

// Store is a PostgreSQL-backed storage.Store with LISTEN/NOTIFY fan-out.
type Store struct {
	db       *bun.DB
	notifier *Notifier
}

var _ storage.Store = (*Store)(nil)

// Open connects with Bun + pgdriver, runs migrations, and starts the event notifier listener.
func Open(ctx context.Context, dsn string) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("postgres: empty dsn")
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	sqldb.SetMaxOpenConns(32)
	sqldb.SetMaxIdleConns(8)

	db := bun.NewDB(sqldb, pgdialect.New())
	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	n, err := NewNotifier(db, dsn, localInstanceID())
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db, notifier: n}, nil
}

// Notifier returns the store's EventNotifier (same instance receives NOTIFY and exposes Listen).
func (s *Store) Notifier() storage.EventNotifier { return s.notifier }

// Close closes the notifier and database pool.
func (s *Store) Close() error {
	var err1, err2 error
	if s.notifier != nil {
		err1 = s.notifier.Close()
	}
	if s.db != nil {
		err2 = s.db.Close()
	}
	return errors.Join(err1, err2)
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

// eventTagInsert maps full_json to JSONB for PostgreSQL.
type eventTagInsert struct {
	bun.BaseModel `bun:"table:event_tags,alias:et"`

	EventID  string `bun:"event_id,notnull"`
	Pos      int    `bun:"pos,notnull"`
	Name     string `bun:"name,notnull"`
	Value    string `bun:"value,notnull"`
	FullJSON string `bun:"full_json,notnull,type:jsonb"`
}

// SaveEvent persists an event, replacing prior replaceable/addressable rows per NIP-01.
func (s *Store) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	if nostr.IsEphemeral(ev.Kind) {
		return errors.New("postgres: ephemeral events are not stored")
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
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
			tag := eventTagInsert{
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
	return applyFilterQueryPrefix(q, f, "")
}

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
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*storage.EventTagRow)(nil)).Where("event_id = ?", id).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewDelete().Model((*storage.EventRow)(nil)).Where("id = ?", id).Exec(ctx)
		return err
	})
}

// CountEvents returns how many distinct events match any filter (OR).
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

// SearchEvents uses tsvector + GIN (NIP-50), ordered by ts_rank_cd descending.
func (s *Store) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	q := strings.TrimSpace(searchQuery)
	if q == "" {
		return nil, nil
	}
	cons := constraints.WithoutSearch()

	var rows []storage.EventRow
	sel := s.db.NewSelect().Model(&rows)
	sel = sel.Where("search_vector @@ websearch_to_tsquery('english', ?)", q)
	sel = sel.OrderExpr("ts_rank_cd(search_vector, websearch_to_tsquery('english', ?)) DESC", q)
	sel = applyFilterQueryPrefix(sel, &cons, "")
	lim := filterLimit(&cons, true)
	if lim < math.MaxInt32 {
		sel = sel.Limit(lim)
	}
	if err := sel.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]*nostr.Event, 0, len(rows))
	for i := range rows {
		ev, err := s.rowToEvent(ctx, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
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
		Where("LOWER(SUBSTR(id, 1, 8)) = ?", p)
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
		Where("pubkey = ?", relayPubkey).
		Where("kind IN (?, ?)", 9000, 9001).
		Where("id IN (SELECT event_id FROM event_tags WHERE name = 'h' AND value = ?)", groupID).
		Where("id IN (SELECT event_id FROM event_tags WHERE name = 'p' AND value = ?)", memberPubkey).
		Order("created_at DESC", "id ASC").
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
	row := storage.AuditLogRow{
		CreatedAt: e.CreatedAt,
		Action:    e.Action,
		Detail:    e.Detail,
		Pubkey:    e.Pubkey,
	}
	_, err := s.db.NewInsert().Model(&row).Exec(ctx)
	return err
}

func (s *Store) QueryAuditLog(ctx context.Context, query storage.AuditQuery) ([]storage.AuditEntry, error) {
	var rows []storage.AuditLogRow
	q := s.db.NewSelect().Model(&rows).Order("created_at DESC")
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

func (s *Store) PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error) {
	res, err := s.db.NewDelete().Model((*storage.AuditLogRow)(nil)).
		Where("created_at < ?", olderThanUnix).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return n, err
}

func (s *Store) SaveConfigChange(ctx context.Context, c storage.ConfigChange) error {
	row := storage.ConfigChangelogRow{
		CreatedAt: c.CreatedAt,
		Summary:   c.Summary,
		JSONDiff:  c.JSONDiff,
	}
	_, err := s.db.NewInsert().Model(&row).Exec(ctx)
	return err
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
