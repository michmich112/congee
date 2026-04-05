package postgres

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

const schemaVersion = 1

func runMigrations(ctx context.Context, db *bun.DB) error {
	var evExists bool
	q := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'events'
	)`
	if err := db.QueryRowContext(ctx, q).Scan(&evExists); err != nil {
		return fmt.Errorf("postgres: check events table: %w", err)
	}
	if evExists {
		var version int
		err := db.QueryRowContext(ctx, `SELECT version FROM congee_schema_version WHERE id = 1`).Scan(&version)
		if err != nil {
			return fmt.Errorf("postgres: read schema version: %w", err)
		}
		if version < schemaVersion {
			return fmt.Errorf("postgres: unsupported schema version %d (need %d)", version, schemaVersion)
		}
		return nil
	}

	stmts := []string{
		`CREATE TABLE congee_schema_version (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			version INT NOT NULL
		)`,
		`INSERT INTO congee_schema_version (id, version) VALUES (1, $1)`,
		`CREATE TABLE events (
			id VARCHAR(128) NOT NULL PRIMARY KEY,
			pubkey VARCHAR(128) NOT NULL,
			created_at BIGINT NOT NULL,
			kind INT NOT NULL,
			content TEXT NOT NULL,
			sig VARCHAR(256) NOT NULL,
			d_tag TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX idx_events_pubkey_kind ON events (pubkey, kind)`,
		`CREATE INDEX idx_events_pubkey_kind_dtag ON events (pubkey, kind, d_tag)`,
		`CREATE INDEX idx_events_created_at ON events (created_at DESC)`,
		`CREATE TABLE event_tags (
			id BIGSERIAL PRIMARY KEY,
			event_id VARCHAR(128) NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			pos INT NOT NULL,
			name TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			full_json JSONB NOT NULL
		)`,
		`CREATE INDEX idx_event_tags_event_id ON event_tags (event_id)`,
		`CREATE INDEX idx_event_tags_name_value ON event_tags (name, value)`,
		`CREATE TABLE audit_log (
			id BIGSERIAL PRIMARY KEY,
			created_at BIGINT NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX idx_audit_created_at ON audit_log (created_at DESC)`,
		`CREATE TABLE config_changelog (
			id BIGSERIAL PRIMARY KEY,
			created_at BIGINT NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`CREATE INDEX idx_config_changelog_created_at ON config_changelog (created_at DESC)`,
	}

	for i, s := range stmts {
		if i == 1 {
			if _, err := db.ExecContext(ctx, s, schemaVersion); err != nil {
				return fmt.Errorf("postgres: migrate: %w", err)
			}
			continue
		}
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("postgres: migrate: %w", err)
		}
	}
	return nil
}
