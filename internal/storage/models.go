package storage

import (
	"github.com/uptrace/bun"
)

// EventRow is the persisted form of a Nostr event.
type EventRow struct {
	bun.BaseModel `bun:"table:events"`

	ID        string `bun:"id,pk"`
	Pubkey    string `bun:"pubkey,notnull"`
	CreatedAt int64  `bun:"created_at,notnull"`
	Kind      int    `bun:"kind,notnull"`
	Content   string `bun:"content,notnull"`
	Sig       string `bun:"sig,notnull"`
	// DTag is the second element of the first "d" tag for addressable kinds; empty otherwise.
	DTag string `bun:"d_tag,notnull,default:''"`
}

// EventTagRow stores one tag array position for indexing (NIP-01 #e / #p / letter tags).
type EventTagRow struct {
	bun.BaseModel `bun:"table:event_tags"`

	ID      int64  `bun:"id,pk,autoincrement"`
	EventID string `bun:"event_id,notnull"`
	Pos     int    `bun:"pos,notnull"`
	Name    string `bun:"name,notnull"`
	// Value is tag[1] when present; used for single-letter tag filters.
	Value string `bun:"value,notnull,default:''"`
	// Full holds JSON array of all strings in the tag (including name).
	FullJSON string `bun:"full_json,notnull"`
}

// AuditLogRow is one relay audit entry.
type AuditLogRow struct {
	bun.BaseModel `bun:"table:audit_log"`

	ID        int64  `bun:"id,pk,autoincrement"`
	CreatedAt int64  `bun:"created_at,notnull"`
	Action    string `bun:"action,notnull"`
	Detail    string `bun:"detail"`
	Pubkey    string `bun:"pubkey,notnull,default:''"`
}

// ConfigChangelogRow records admin config changes.
type ConfigChangelogRow struct {
	bun.BaseModel `bun:"table:config_changelog"`

	ID        int64  `bun:"id,pk,autoincrement"`
	CreatedAt int64  `bun:"created_at,notnull"`
	Summary   string `bun:"summary,notnull"`
	JSONDiff  string `bun:"json_diff,notnull"`
}
