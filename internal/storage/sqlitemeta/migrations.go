package sqlitemeta

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

const schemaVersion = 1

// CurrentSchemaVersion is the PRAGMA user_version / app-expected value for this binary.
func CurrentSchemaVersion() int { return schemaVersion }

func runMigrations(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	var version int
	row := db.QueryRowContext(ctx, "PRAGMA user_version")
	if err := row.Scan(&version); err != nil {
		return fmt.Errorf("sqlitemeta: read user_version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("sqlitemeta: unsupported schema version %d (need <= %d)", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 0 {
		return fmt.Errorf("sqlitemeta: unsupported schema version %d", version)
	}
	return migrateFresh(ctx, db, log)
}

func migrateFresh(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_log (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_pubkey_created_at ON audit_log (pubkey, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS config_changelog (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_config_changelog_created_at ON config_changelog (created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS relay_metric_buckets (
			bucket_start_unix INTEGER NOT NULL PRIMARY KEY,
			events_stored INTEGER NOT NULL DEFAULT 0,
			events_rejected INTEGER NOT NULL DEFAULT 0,
			req_count INTEGER NOT NULL DEFAULT 0,
			close_count INTEGER NOT NULL DEFAULT 0,
			query_ms_sum INTEGER NOT NULL DEFAULT 0,
			query_ms_count INTEGER NOT NULL DEFAULT 0,
			subscriptions_open INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_relay_metric_buckets_start ON relay_metric_buckets (bucket_start_unix)`,
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
		log.Debug().Int("ddl_step", i).Msg("schema: exec ddl statement")
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			return fmt.Errorf("sqlitemeta: migrate: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("sqlitemeta: set user_version: %w", err)
	}
	return nil
}
