package storage

import (
	"context"
	"encoding/json"

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
	// ConnID, when set, matches audit rows whose detail contains "conn_id=<8-hex>" for that connection.
	ConnID string
	// Kinds, when non-empty, matches audit rows whose detail ends with " kind=<n>" (NIP-01 post-hook format)
	// for any n in the list (OR semantics). Values are deduped and sorted by the admin API layer.
	Kinds []int
}

// WSConnectionSession is one closed WebSocket client session (admin audit UI).
type WSConnectionSession struct {
	ID               int64           `json:"id"`
	ConnID           string          `json:"conn_id"`
	PeerIP           string          `json:"peer_ip"`
	RemoteAddr       string          `json:"remote_addr"`
	StartedUnix      int64           `json:"started_unix"`
	EndedUnix        int64           `json:"ended_unix"`
	TotalAuth        int64           `json:"total_auth"`
	TotalReq         int64           `json:"total_req"`
	TotalClientEvent int64           `json:"total_client_event"`
	SeriesJSON       json.RawMessage `json:"series_json"`
	SubsJSON         json.RawMessage `json:"subs_json"`
}

// WSConnectionSessionQuery lists closed sessions newest-first.
type WSConnectionSessionQuery struct {
	Limit  int
	Offset int
}

// EventStore persists and queries Nostr events (and related relay data such as NIP-29 lookups).
type EventStore interface {
	SaveEvent(ctx context.Context, ev *nostr.Event) error
	QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error)
	DeleteEvent(ctx context.Context, id string) error
	// CountEvents returns the number of distinct stored events matching any of the filters (OR).
	// When filters is nil or empty, implementations run COUNT(*) over all events (used by health
	// checks to verify the database is reachable). Filters that only carry NIP-50 search text are
	// ignored for counting, matching QueryEvents behavior for those filters.
	CountEvents(ctx context.Context, filters []nostr.Filter) (int, error)
	// HasEventID reports whether an event with the given id is already stored.
	HasEventID(ctx context.Context, id string) (bool, error)
	// SearchEvents runs a NIP-50 full-text query against stored event content, intersected
	// with structural constraints from constraints (ids, authors, kinds, time range, tags).
	// The Search field on constraints is ignored.
	SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error)

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

// MetaStore persists relay operational metadata (audit, metrics, config changelog, WS sessions).
type MetaStore interface {
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

	// SaveWSConnectionSession inserts one closed WebSocket session row; returns the new row id.
	SaveWSConnectionSession(ctx context.Context, s WSConnectionSession) (int64, error)
	QueryWSConnectionSessions(ctx context.Context, q WSConnectionSessionQuery) ([]WSConnectionSession, error)
	// CountWSConnectionSessions returns the number of persisted closed WebSocket sessions (all rows).
	CountWSConnectionSessions(ctx context.Context) (int64, error)
	// GetWSConnectionSessionByID returns one closed session by primary key, or nil when not found.
	GetWSConnectionSessionByID(ctx context.Context, id int64) (*WSConnectionSession, error)
	// PurgeWSConnectionSessionsBefore deletes rows with ended_unix < olderThanUnix (exclusive).
	PurgeWSConnectionSessionsBefore(ctx context.Context, olderThanUnix int64) (int64, error)

	SaveConfigChange(ctx context.Context, c ConfigChange) error
	QueryConfigChangelog(ctx context.Context, limit int) ([]ConfigChange, error)

	// UpsertRelayMetricBucket writes or replaces one UTC-minute aggregate row.
	UpsertRelayMetricBucket(ctx context.Context, b RelayMetricBucket) error
	// QueryRelayMetricBuckets returns buckets with bucket_start_unix >= MinBucketStartUnix ordered ascending, capped by Limit.
	QueryRelayMetricBuckets(ctx context.Context, q RelayMetricBucketQuery) ([]RelayMetricBucket, error)
	// PurgeRelayMetricBucketsBefore deletes rows with bucket_start_unix < cutoffStartUnixExclusive.
	PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error)
}

// Store is the full relay persistence API (events plus operational metadata).
type Store interface {
	EventStore
	MetaStore
	// AdminStorageSnapshot returns approximate table row counts (sqlite_stat1 / pg stats) and on-disk bytes (SQLite file+WAL+SHM, Postgres pg_database_size).
	AdminStorageSnapshot(ctx context.Context) (AdminStorageSnapshot, error)
}
