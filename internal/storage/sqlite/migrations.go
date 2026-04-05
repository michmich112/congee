package sqlite

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

const schemaVersion = 2

func runMigrations(ctx context.Context, db *bun.DB) error {
	var version int
	row := db.QueryRowContext(ctx, "PRAGMA user_version")
	if err := row.Scan(&version); err != nil {
		return fmt.Errorf("sqlite: read user_version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("sqlite: unsupported schema version %d (need <= %d)", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}

	if version == 0 {
		if err := migrateFresh(ctx, db); err != nil {
			return err
		}
		return nil
	}
	if version == 1 {
		return migrateV1ToV2(ctx, db)
	}
	return fmt.Errorf("sqlite: unsupported schema version %d", version)
}

func migrateFresh(ctx context.Context, db *bun.DB) error {
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
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_log (created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS config_changelog (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_config_changelog_created_at ON config_changelog (created_at DESC)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite: migrate: %w", err)
		}
	}
	if err := createFTS5AndTriggers(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("sqlite: set user_version: %w", err)
	}
	return nil
}

func migrateV1ToV2(ctx context.Context, db *bun.DB) error {
	if err := createFTS5AndTriggers(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO event_fts(event_id, content) SELECT id, content FROM events`); err != nil {
		return fmt.Errorf("sqlite: backfill event_fts: %w", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("sqlite: set user_version: %w", err)
	}
	return nil
}

func createFTS5AndTriggers(ctx context.Context, db *bun.DB) error {
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
	for _, s := range fts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite: fts5: %w", err)
		}
	}
	return nil
}
