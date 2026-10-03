package sqlevent

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

const schemaVersion = 8

// CurrentSchemaVersion is the PRAGMA user_version / app-expected value for this binary.
func CurrentSchemaVersion() int { return schemaVersion }

// RunMigrations applies schema DDL until PRAGMA user_version reaches CurrentSchemaVersion.
func RunMigrations(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	return runMigrations(ctx, db, engine, log)
}

func runMigrations(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	for {
		var version int
		row := db.QueryRowContext(ctx, "PRAGMA user_version")
		if err := row.Scan(&version); err != nil {
			return fmt.Errorf("%s: read user_version: %w", engine, err)
		}
		log.Debug().Int("user_version", version).Msg("schema: read user_version")
		if version > schemaVersion {
			return fmt.Errorf("%s: unsupported schema version %d (need <= %d)", engine, version, schemaVersion)
		}
		if version == schemaVersion {
			log.Debug().Msg("schema: already at current version")
			return nil
		}
		if version == 0 {
			log.Debug().Msg("schema: user_version 0; applying fresh schema")
			if err := migrateFresh(ctx, db, engine, log); err != nil {
				return err
			}
			log.Debug().Msg("schema: fresh schema applied")
			return nil
		}
		switch version {
		case 1:
			log.Debug().Msg("schema: migrating v1 to v2")
			if err := migrateV1ToV2(ctx, db, engine, log); err != nil {
				return err
			}
		case 2:
			log.Debug().Msg("schema: migrating v2 to v3")
			if err := migrateV2ToV3(ctx, db, engine, log); err != nil {
				return err
			}
		case 3:
			log.Debug().Msg("schema: migrating v3 to v4")
			if err := migrateV3ToV4(ctx, db, engine, log); err != nil {
				return err
			}
		case 4:
			log.Debug().Msg("schema: migrating v4 to v5")
			if err := migrateV4ToV5(ctx, db, engine, log); err != nil {
				return err
			}
		case 5:
			log.Debug().Msg("schema: migrating v5 to v6")
			if err := migrateV5ToV6(ctx, db, engine, log); err != nil {
				return err
			}
		case 6:
			log.Debug().Msg("schema: migrating v6 to v7")
			if err := migrateV6ToV7(ctx, db, engine, log); err != nil {
				return err
			}
		case 7:
			log.Debug().Msg("schema: migrating v7 to v8")
			if err := migrateV7ToV8(ctx, db, engine, log); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unsupported schema version %d", engine, version)
		}
	}
}

func migrateFresh(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT NOT NULL PRIMARY KEY,
			pubkey TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			kind INTEGER NOT NULL,
			content TEXT NOT NULL,
			sig TEXT NOT NULL,
			d_tag TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_pubkey_kind ON events (pubkey, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_events_pubkey_kind_dtag ON events (pubkey, kind, d_tag)`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS event_tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			name TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			full_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_tags_event_id ON event_tags (event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_tags_name_value ON event_tags (name, value)`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema: exec ddl statement")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("%s: migrate: %w", engine, err)
		}
	}
	log.Debug().Msg("schema: creating turso fts index")
	if err := createTursoContentFTS(ctx, db, engine); err != nil {
		return err
	}
	log.Debug().Int("schema_version", schemaVersion).Msg("schema: set user_version")
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func migrateV1ToV2(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	log.Debug().Msg("schema v1->v2: skip fts5; turso fts is created at v8")
	return migrateV2ToV3(ctx, db, engine, log)
}

func migrateV2ToV3(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS relay_metric_buckets (
			bucket_start_unix INTEGER NOT NULL PRIMARY KEY,
			events_stored INTEGER NOT NULL DEFAULT 0,
			events_rejected INTEGER NOT NULL DEFAULT 0,
			req_count INTEGER NOT NULL DEFAULT 0,
			close_count INTEGER NOT NULL DEFAULT 0,
			query_ms_sum INTEGER NOT NULL DEFAULT 0,
			query_ms_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_relay_metric_buckets_start ON relay_metric_buckets (bucket_start_unix)`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema v2->v3: relay_metric_buckets")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("%s: migrate v2->v3: %w", engine, err)
		}
	}
	log.Debug().Int("schema_version", schemaVersion).Msg("schema v2->v3: set user_version 3 (chain v3->v4)")
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 3`); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	log.Debug().Msg("schema v2->v3: chain v3->v4")
	return migrateV3ToV4(ctx, db, engine, log)
}

func migrateV3ToV4(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	var colCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('relay_metric_buckets') WHERE name = 'subscriptions_open'`,
	).Scan(&colCount); err != nil {
		return fmt.Errorf("%s: migrate v3->v4: %w", engine, err)
	}
	if colCount == 0 {
		log.Debug().Msg("schema v3->v4: subscriptions_open on relay_metric_buckets")
		if _, err := db.ExecContext(ctx, `ALTER TABLE relay_metric_buckets ADD COLUMN subscriptions_open INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("%s: migrate v3->v4: %w", engine, err)
		}
	} else {
		log.Debug().Msg("schema v3->v4: subscriptions_open already present; skipping alter")
	}
	log.Debug().Msg("schema v3->v4: set user_version 4")
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 4`); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func migrateV4ToV5(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_event_tags_name_value_event_id ON event_tags (name, value, event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_tags_event_id_pos ON event_tags (event_id, pos)`,
		`DROP INDEX IF EXISTS idx_event_tags_event_id`,
		`CREATE INDEX IF NOT EXISTS idx_audit_pubkey_created_at ON audit_log (pubkey, created_at DESC)`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema v4->v5: exec ddl statement")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("%s: migrate v4->v5: %w", engine, err)
		}
	}
	log.Debug().Msg("schema v4->v5: set user_version 5")
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 5`); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func migrateV5ToV6(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ws_connection_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conn_id TEXT NOT NULL,
			peer_ip TEXT NOT NULL,
			remote_addr TEXT NOT NULL,
			started_unix INTEGER NOT NULL,
			ended_unix INTEGER NOT NULL,
			total_req INTEGER NOT NULL DEFAULT 0,
			total_client_event INTEGER NOT NULL DEFAULT 0,
			series_json TEXT NOT NULL DEFAULT '[]',
			subs_json TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ws_sessions_ended ON ws_connection_sessions (ended_unix DESC)`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema v5->v6: ws_connection_sessions")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("%s: migrate v5->v6: %w", engine, err)
		}
	}
	log.Debug().Msg("schema v5->v6: set user_version 6")
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 6`); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func migrateV6ToV7(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	stmts := []string{
		`DROP INDEX IF EXISTS idx_ws_sessions_ended`,
		`DROP TABLE IF EXISTS ws_connection_sessions`,
		`DROP INDEX IF EXISTS idx_relay_metric_buckets_start`,
		`DROP TABLE IF EXISTS relay_metric_buckets`,
		`DROP INDEX IF EXISTS idx_config_changelog_created_at`,
		`DROP TABLE IF EXISTS config_changelog`,
		`DROP INDEX IF EXISTS idx_audit_pubkey_created_at`,
		`DROP INDEX IF EXISTS idx_audit_created_at`,
		`DROP TABLE IF EXISTS audit_log`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema v6->v7: drop meta tables")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("%s: migrate v6->v7: %w", engine, err)
		}
	}
	log.Debug().Msg("schema v6->v7: set user_version 7")
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 7`); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func migrateV7ToV8(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	var hasEvents int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='events'`).Scan(&hasEvents); err != nil {
		return fmt.Errorf("%s: migrate v7->v8: %w", engine, err)
	}
	if hasEvents == 0 {
		log.Debug().Msg("schema v7->v8: no events table; set user_version 8")
		if _, err := db.ExecContext(ctx, `PRAGMA user_version = 8`); err != nil {
			return fmt.Errorf("%s: set user_version: %w", engine, err)
		}
		return nil
	}
	var ftsLeft int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE name = 'event_fts' OR name IN ('events_ai_fts', 'events_au_fts', 'events_ad_fts')`).Scan(&ftsLeft); err != nil {
		return fmt.Errorf("%s: migrate v7->v8: %w", engine, err)
	}
	if ftsLeft > 0 {
		return fmt.Errorf("%s: migrate v7->v8: fts5 objects still present", engine)
	}
	before, err := eventAggregates(ctx, db)
	if err != nil {
		return fmt.Errorf("%s: migrate v7->v8: %w", engine, err)
	}
	if _, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS event_content_fts`); err != nil {
		return fmt.Errorf("%s: migrate v7->v8: drop index: %w", engine, err)
	}
	log.Debug().Msg("schema v7->v8: create turso fts index")
	if err := createTursoContentFTS(ctx, db, engine); err != nil {
		return err
	}
	after, err := eventAggregates(ctx, db)
	if err != nil {
		return fmt.Errorf("%s: migrate v7->v8: %w", engine, err)
	}
	if before != after {
		return fmt.Errorf("%s: migrate v7->v8: event aggregates changed", engine)
	}
	if before.events > 0 {
		if err := verifyFTSBackfill(ctx, db, engine); err != nil {
			return err
		}
	}
	log.Debug().Msg("schema v7->v8: set user_version 8")
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 8`); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

type agg struct {
	events       int64
	tags         int64
	contentBytes int64
}

func eventAggregates(ctx context.Context, db *bun.DB) (agg, error) {
	var a agg
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&a.events); err != nil {
		return a, err
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_tags`).Scan(&a.tags); err != nil {
		return a, err
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(content)), 0) FROM events`).Scan(&a.contentBytes); err != nil {
		return a, err
	}
	return a, nil
}

func verifyFTSBackfill(ctx context.Context, db *bun.DB, engine string) error {
	rows, err := db.QueryContext(ctx, `SELECT id, content FROM events`)
	if err != nil {
		return fmt.Errorf("%s: migrate v7->v8: read events: %w", engine, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return err
		}
		tokens := indexableTokens(content)
		if len(tokens) == 0 {
			continue
		}
		matched := false
		for _, token := range tokens {
			phrase := `"` + strings.ReplaceAll(token, `"`, `""`) + `"`
			var n int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE id = ? AND fts_match(content, ?)`, id, phrase).Scan(&n); err != nil {
				return fmt.Errorf("%s: migrate v7->v8: fts backfill: %w", engine, err)
			}
			if n == 1 {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: migrate v7->v8: existing event %s was not indexed", engine, id)
		}
		return nil
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

// indexableTokens returns tokens the Turso default FTS tokenizer would keep:
// Unicode letters and digits, lowercased, at most 40 bytes. ASCII-only slicing
// turns "café" into "caf", which is not an indexed token and fails this check.
func indexableTokens(content string) []string {
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := strings.ToLower(b.String())
		b.Reset()
		if len(tok) > 40 {
			return
		}
		tokens = append(tokens, tok)
	}
	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func createTursoContentFTS(ctx context.Context, db *bun.DB, engine string) error {
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS event_content_fts ON events USING fts (content)`); err != nil {
		return fmt.Errorf("%s: turso fts: %w", engine, err)
	}
	return nil
}
