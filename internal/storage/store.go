package storage

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// AuditEntry is a row written to the audit log.
type AuditEntry struct {
	CreatedAt int64
	Action    string
	Detail    string
	Pubkey    string
}

// ConfigChange is one entry in the config changelog.
type ConfigChange struct {
	CreatedAt int64
	Summary   string
	JSONDiff  string
}

// Store is the relay persistence API (SQLite, PostgreSQL, etc.).
type Store interface {
	SaveEvent(ctx context.Context, ev *nostr.Event) error
	QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error)
	DeleteEvent(ctx context.Context, id string) error
	CountEvents(ctx context.Context, filters []nostr.Filter) (int, error)
	SearchEvents(ctx context.Context, query string, limit int) ([]*nostr.Event, error)

	SaveAuditEntry(ctx context.Context, e AuditEntry) error
	QueryAuditLog(ctx context.Context, since, until int64, limit int) ([]AuditEntry, error)
	PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error)

	SaveConfigChange(ctx context.Context, c ConfigChange) error
	QueryConfigChangelog(ctx context.Context, limit int) ([]ConfigChange, error)
}
