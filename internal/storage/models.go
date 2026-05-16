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

// RelayMetricBucketRow is one persisted UTC-minute relay telemetry bucket.
type RelayMetricBucketRow struct {
	bun.BaseModel `bun:"table:relay_metric_buckets"`

	BucketStartUnix int64 `bun:"bucket_start_unix,pk"`
	EventsStored    int64 `bun:"events_stored,notnull"`
	EventsRejected  int64 `bun:"events_rejected,notnull"`
	ReqCount        int64 `bun:"req_count,notnull"`
	CloseCount      int64 `bun:"close_count,notnull"`
	QueryMsSum          int64 `bun:"query_ms_sum,notnull"`
	QueryMsCount        int64 `bun:"query_ms_count,notnull"`
	SubscriptionsOpen   int64 `bun:"subscriptions_open,notnull"`
}

// WSConnectionSessionRow is one closed WebSocket relay session (admin connections audit).
type WSConnectionSessionRow struct {
	bun.BaseModel `bun:"table:ws_connection_sessions"`

	ID               int64  `bun:"id,pk,autoincrement"`
	ConnID           string `bun:"conn_id,notnull"`
	PeerIP           string `bun:"peer_ip,notnull"`
	RemoteAddr       string `bun:"remote_addr,notnull"`
	StartedUnix      int64  `bun:"started_unix,notnull"`
	EndedUnix        int64  `bun:"ended_unix,notnull"`
	TotalReq         int64  `bun:"total_req,notnull"`
	TotalClientEvent int64  `bun:"total_client_event,notnull"`
	SeriesJSON       string `bun:"series_json,notnull"`
	SubsJSON         string `bun:"subs_json,notnull"`
}
