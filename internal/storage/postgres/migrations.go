package postgres

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

const schemaVersion = 7

// CurrentSchemaVersion is the congee_schema_version / app-expected value for this binary.
func CurrentSchemaVersion() int { return schemaVersion }

func runMigrations(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	var evExists bool
	q := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'events'
	)`
	if err := db.QueryRowContext(ctx, q).Scan(&evExists); err != nil {
		return fmt.Errorf("postgres: check events table: %w", err)
	}
	log.Debug().Bool("events_table_exists", evExists).Msg("schema: checked events table")
	if !evExists {
		log.Debug().Msg("schema: no events table; applying fresh schema")
		if err := migrateFresh(ctx, db, log); err != nil {
			return err
		}
		log.Debug().Msg("schema: fresh schema applied")
		return nil
	}

	for {
		var version int
		err := db.QueryRowContext(ctx, `SELECT version FROM congee_schema_version WHERE id = 1`).Scan(&version)
		if err != nil {
			return fmt.Errorf("postgres: read schema version: %w", err)
		}
		log.Debug().Int("schema_version", version).Msg("schema: read version row")
		if version > schemaVersion {
			return fmt.Errorf("postgres: unsupported schema version %d (need <= %d)", version, schemaVersion)
		}
		if version == schemaVersion {
			log.Debug().Msg("schema: already at current version")
			return nil
		}
		switch version {
		case 1:
			log.Debug().Msg("schema: migrating v1 to v2")
			if err := migrateV1ToV2(ctx, db, log); err != nil {
				return err
			}
		case 2:
			log.Debug().Msg("schema: migrating v2 to v3")
			if err := migrateV2ToV3(ctx, db, log); err != nil {
				return err
			}
		case 3:
			log.Debug().Msg("schema: migrating v3 to v4")
			if err := migrateV3ToV4(ctx, db, log); err != nil {
				return err
			}
		case 4:
			log.Debug().Msg("schema: migrating v4 to v5")
			if err := migrateV4ToV5(ctx, db, log); err != nil {
				return err
			}
		case 5:
			log.Debug().Msg("schema: migrating v5 to v6")
			if err := migrateV5ToV6(ctx, db, log); err != nil {
				return err
			}
		case 6:
			log.Debug().Msg("schema: migrating v6 to v7")
			if err := migrateV6ToV7(ctx, db, log); err != nil {
				return err
			}
		default:
			return fmt.Errorf("postgres: unsupported schema version %d", version)
		}
	}
}

func migrateFresh(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	// IF NOT EXISTS / upsert: targets may be half-applied after a failed migrate (e.g. only
	// congee_schema_version exists while events is still missing).
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS congee_schema_version (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			version INT NOT NULL
		)`,
		`INSERT INTO congee_schema_version (id, version) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version`,
		`CREATE TABLE IF NOT EXISTS events (
			id VARCHAR(128) NOT NULL PRIMARY KEY,
			pubkey VARCHAR(128) NOT NULL,
			created_at BIGINT NOT NULL,
			kind INT NOT NULL,
			content TEXT NOT NULL,
			sig VARCHAR(256) NOT NULL,
			d_tag TEXT NOT NULL DEFAULT '',
			search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_pubkey_kind ON events (pubkey, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_events_pubkey_kind_dtag ON events (pubkey, kind, d_tag)`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_search_vector ON events USING GIN (search_vector)`,
		`CREATE TABLE IF NOT EXISTS event_tags (
			id BIGSERIAL PRIMARY KEY,
			event_id VARCHAR(128) NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			pos INT NOT NULL,
			name TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			full_json JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_tags_event_id ON event_tags (event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_tags_name_value ON event_tags (name, value)`,
	}

	for i, s := range stmts {
		if i == 1 {
			log.Debug().Int("ddl_step", i).Msg("schema: upsert congee_schema_version")
			if _, err := db.ExecContext(ctx, s, schemaVersion); err != nil {
				return fmt.Errorf("postgres: migrate: %w", err)
			}
			continue
		}
		log.Debug().Int("ddl_step", i).Msg("schema: exec ddl statement")
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("postgres: migrate: %w", err)
		}
	}
	return nil
}

func migrateV1ToV2(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	stmts := []string{
		`ALTER TABLE events ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED`,
		`CREATE INDEX IF NOT EXISTS idx_events_search_vector ON events USING GIN (search_vector)`,
		`UPDATE congee_schema_version SET version = ? WHERE id = 1`,
	}
	log.Debug().Int("step", 0).Msg("schema v1->v2: add search_vector column")
	if _, err := db.ExecContext(ctx, stmts[0]); err != nil {
		return fmt.Errorf("postgres: add search_vector: %w", err)
	}
	log.Debug().Int("step", 1).Msg("schema v1->v2: create gin index")
	if _, err := db.ExecContext(ctx, stmts[1]); err != nil {
		return fmt.Errorf("postgres: gin index: %w", err)
	}
	log.Debug().Int("step", 2).Msg("schema v1->v2: bump schema version to 2")
	if _, err := db.ExecContext(ctx, stmts[2], 2); err != nil {
		return fmt.Errorf("postgres: bump schema version: %w", err)
	}
	log.Debug().Msg("schema v1->v2: chain v2->v3")
	return migrateV2ToV3(ctx, db, log)
}

func migrateV2ToV3(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS relay_metric_buckets (
			bucket_start_unix BIGINT NOT NULL PRIMARY KEY,
			events_stored BIGINT NOT NULL DEFAULT 0,
			events_rejected BIGINT NOT NULL DEFAULT 0,
			req_count BIGINT NOT NULL DEFAULT 0,
			close_count BIGINT NOT NULL DEFAULT 0,
			query_ms_sum BIGINT NOT NULL DEFAULT 0,
			query_ms_count BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_relay_metric_buckets_start ON relay_metric_buckets (bucket_start_unix)`,
		`UPDATE congee_schema_version SET version = ? WHERE id = 1`,
	}
	for i := 0; i < 2; i++ {
		log.Debug().Int("ddl_step", i).Msg("schema v2->v3: relay_metric_buckets")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("postgres: migrate v2->v3: %w", err)
		}
	}
	log.Debug().Msg("schema v2->v3: bump schema version to 3 (chain v3->v4)")
	if _, err := db.ExecContext(ctx, stmts[2], 3); err != nil {
		return fmt.Errorf("postgres: bump schema version: %w", err)
	}
	return migrateV3ToV4(ctx, db, log)
}

func migrateV3ToV4(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	log.Debug().Msg("schema v3->v4: subscriptions_open on relay_metric_buckets")
	if _, err := db.ExecContext(ctx, `ALTER TABLE relay_metric_buckets ADD COLUMN IF NOT EXISTS subscriptions_open BIGINT NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("postgres: migrate v3->v4: %w", err)
	}
	log.Debug().Msg("schema v3->v4: bump schema version to 4")
	if _, err := db.ExecContext(ctx, `UPDATE congee_schema_version SET version = ? WHERE id = 1`, 4); err != nil {
		return fmt.Errorf("postgres: bump schema version: %w", err)
	}
	return nil
}

func migrateV4ToV5(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_event_tags_name_value_event_id ON event_tags (name, value, event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_tags_event_id_pos ON event_tags (event_id, pos)`,
		`DROP INDEX IF EXISTS idx_event_tags_event_id`,
		`CREATE INDEX IF NOT EXISTS idx_audit_pubkey_created_at ON audit_log (pubkey, created_at DESC)`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema v4->v5: exec ddl statement")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("postgres: migrate v4->v5: %w", err)
		}
	}
	log.Debug().Msg("schema v4->v5: bump schema version to 5")
	if _, err := db.ExecContext(ctx, `UPDATE congee_schema_version SET version = ? WHERE id = 1`, 5); err != nil {
		return fmt.Errorf("postgres: bump schema version: %w", err)
	}
	return nil
}

func migrateV5ToV6(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ws_connection_sessions (
			id BIGSERIAL PRIMARY KEY,
			conn_id TEXT NOT NULL,
			peer_ip TEXT NOT NULL,
			remote_addr TEXT NOT NULL,
			started_unix BIGINT NOT NULL,
			ended_unix BIGINT NOT NULL,
			total_req BIGINT NOT NULL DEFAULT 0,
			total_client_event BIGINT NOT NULL DEFAULT 0,
			series_json TEXT NOT NULL DEFAULT '[]',
			subs_json TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ws_sessions_ended ON ws_connection_sessions (ended_unix DESC)`,
	}
	for i := range stmts {
		log.Debug().Int("ddl_step", i).Msg("schema v5->v6: ws_connection_sessions")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("postgres: migrate v5->v6: %w", err)
		}
	}
	log.Debug().Msg("schema v5->v6: bump schema version to 6")
	if _, err := db.ExecContext(ctx, `UPDATE congee_schema_version SET version = ? WHERE id = 1`, 6); err != nil {
		return fmt.Errorf("postgres: bump schema version: %w", err)
	}
	return nil
}

func migrateV6ToV7(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
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
			return fmt.Errorf("postgres: migrate v6->v7: %w", err)
		}
	}
	log.Debug().Msg("schema v6->v7: bump schema version to 7")
	if _, err := db.ExecContext(ctx, `UPDATE congee_schema_version SET version = ? WHERE id = 1`, schemaVersion); err != nil {
		return fmt.Errorf("postgres: bump schema version: %w", err)
	}
	return nil
}
