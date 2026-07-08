package sqlevent

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

const schemaVersion = 7

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
	log.Debug().Msg("schema: creating fts5 and triggers")
	if err := createFTS5AndTriggers(ctx, db, engine, log); err != nil {
		return err
	}
	log.Debug().Int("schema_version", schemaVersion).Msg("schema: set user_version")
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func migrateV1ToV2(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	log.Debug().Msg("schema v1->v2: fts5 and triggers")
	if err := createFTS5AndTriggers(ctx, db, engine, log); err != nil {
		return err
	}
	log.Debug().Msg("schema v1->v2: backfill event_fts")
	if _, err := db.ExecContext(ctx, `INSERT INTO event_fts(event_id, content) SELECT id, content FROM events`); err != nil {
		return fmt.Errorf("%s: backfill event_fts: %w", engine, err)
	}
	log.Debug().Msg("schema v1->v2: chain v2->v3")
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
	log.Debug().Int("schema_version", schemaVersion).Msg("schema v6->v7: set user_version")
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("%s: set user_version: %w", engine, err)
	}
	return nil
}

func createFTS5AndTriggers(ctx context.Context, db *bun.DB, engine string, log zerolog.Logger) error {
	fts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS event_fts USING fts5(
			event_id UNINDEXED,
			content,
			tokenize = 'porter unicode61'
		)`,
		`DROP TRIGGER IF EXISTS events_ai_fts`,
		`CREATE TRIGGER events_ai_fts AFTER INSERT ON events BEGIN
			INSERT INTO event_fts(event_id, content) VALUES (new.id, new.content);
		END`,
		`DROP TRIGGER IF EXISTS events_au_fts`,
		`CREATE TRIGGER events_au_fts AFTER UPDATE ON events BEGIN
			DELETE FROM event_fts WHERE event_id = old.id;
			INSERT INTO event_fts(event_id, content) VALUES (new.id, new.content);
		END`,
		`DROP TRIGGER IF EXISTS events_ad_fts`,
		`CREATE TRIGGER events_ad_fts AFTER DELETE ON events BEGIN
			DELETE FROM event_fts WHERE event_id = old.id;
		END`,
	}
	for i := range fts {
		log.Debug().Int("fts_step", i).Msg("schema: fts5/trigger ddl")
		if _, err := db.ExecContext(ctx, fts[i]); err != nil {
			return fmt.Errorf("%s: fts5: %w", engine, err)
		}
	}
	return nil
}
