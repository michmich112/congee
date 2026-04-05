package storage

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// AuditEntry is a row written to the audit log.
type AuditEntry struct {
	CreatedAt int64  `json:"created_at"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	Pubkey    string `json:"pubkey"`
}

// ConfigChange is one entry in the config changelog.
type ConfigChange struct {
	CreatedAt int64  `json:"created_at"`
	Summary   string `json:"summary"`
	JSONDiff  string `json:"json_diff"`
}

// AuditQuery filters and paginates audit_log rows (newest first).
type AuditQuery struct {
	Since  int64
	Until  int64
	Limit  int
	Offset int
	Action string
	Pubkey string
}

// Store is the relay persistence API (SQLite, PostgreSQL, etc.).
type Store interface {
	SaveEvent(ctx context.Context, ev *nostr.Event) error
	QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error)
	DeleteEvent(ctx context.Context, id string) error
	CountEvents(ctx context.Context, filters []nostr.Filter) (int, error)
	SearchEvents(ctx context.Context, query string, limit int) ([]*nostr.Event, error)

	SaveAuditEntry(ctx context.Context, e AuditEntry) error
	QueryAuditLog(ctx context.Context, q AuditQuery) ([]AuditEntry, error)
	PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error)

	SaveConfigChange(ctx context.Context, c ConfigChange) error
	QueryConfigChangelog(ctx context.Context, limit int) ([]ConfigChange, error)
}
