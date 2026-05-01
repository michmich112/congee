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
	// Kinds, when non-empty, matches audit rows whose detail ends with " kind=<n>" (NIP-01 post-hook format)
	// for any n in the list (OR semantics). Values are deduped and sorted by the admin API layer.
	Kinds []int
}

// Store is the relay persistence API (SQLite, PostgreSQL, etc.).
type Store interface {
	SaveEvent(ctx context.Context, ev *nostr.Event) error
	QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error)
	DeleteEvent(ctx context.Context, id string) error
	CountEvents(ctx context.Context, filters []nostr.Filter) (int, error)
	// HasEventID reports whether an event with the given id is already stored.
	HasEventID(ctx context.Context, id string) (bool, error)
	// SearchEvents runs a NIP-50 full-text query against stored event content, intersected
	// with structural constraints from constraints (ids, authors, kinds, time range, tags).
	// The Search field on constraints is ignored.
	SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error)

	SaveAuditEntry(ctx context.Context, e AuditEntry) error
	// HasAuditDuplicate reports whether an audit row identical to e already exists.
	HasAuditDuplicate(ctx context.Context, e AuditEntry) (bool, error)
	QueryAuditLog(ctx context.Context, q AuditQuery) ([]AuditEntry, error)
	// CountAuditLog returns the number of rows matching the same filters as QueryAuditLog
	// (since, until, action, pubkey, kinds). Limit and Offset are ignored.
	CountAuditLog(ctx context.Context, q AuditQuery) (int64, error)
	// ListDistinctAuditKinds returns sorted unique trailing kinds from the newest scanLimit audit rows.
	ListDistinctAuditKinds(ctx context.Context, scanLimit int) ([]int, error)
	PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error)

	SaveConfigChange(ctx context.Context, c ConfigChange) error
	QueryConfigChangelog(ctx context.Context, limit int) ([]ConfigChange, error)

	// NIP-29: EventIDPrefixExists reports whether each 8-hex prefix matches some stored event id.
	// If groupID is non-empty and requireSameH is true, the matching event must carry tag h=groupID.
	EventIDPrefixExists(ctx context.Context, prefix string, groupID string, requireSameH bool) (bool, error)
	// GetLatestGroupMetadata39000 returns the newest kind-39000 addressable row for relayPubkey + group d-tag.
	GetLatestGroupMetadata39000(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error)
	// GetLatestGroupAdmins39001 returns the newest kind-39001 addressable row for relayPubkey + group d-tag.
	GetLatestGroupAdmins39001(ctx context.Context, relayPubkey, groupID string) (*nostr.Event, error)
	// IsGroupMember uses the latest relay-signed kind 9000 or 9001 for the group with p=memberPubkey.
	IsGroupMember(ctx context.Context, relayPubkey, groupID, memberPubkey string) (bool, error)
}
