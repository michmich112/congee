package config

// Config matches the root object in config.example.json.
type Config struct {
	Relay                   RelaySection            `json:"relay"`
	Admin                   AdminSection            `json:"admin"`
	Database                DatabaseSection         `json:"database"`
	Logging                 LoggingSection          `json:"logging"`
	Audit                   AuditSection            `json:"audit"`
	Metrics                 MetricsSection          `json:"metrics"`
	RateLimits              RateLimitsSection       `json:"rate_limits"`
	ConnectionLimits        ConnectionLimitsSection `json:"connection_limits"`
	WebSocket               WebSocketSection        `json:"websocket"`
	MaxSubscriptionIDLength int                     `json:"max_subscription_id_length"`
	NIP11                   NIP11Section            `json:"nip11"`
	NIP42                   NIP42Section            `json:"nip42"`
	NIP29                   NIP29Section            `json:"nip29"`
	NIP17                   NIP17Section            `json:"nip17"`
	NIPs                    NIPsSection             `json:"nips"`
}

type RelaySection struct {
	Port       int    `json:"port"`
	InstanceID string `json:"instance_id,omitempty"`
}

type AdminSection struct {
	Port int `json:"port"`
}

// DefaultDatabaseType is used when database.type is empty in JSON config.
const DefaultDatabaseType = "turso"

type DatabaseSection struct {
	Type    string `json:"type"`
	DSN     string `json:"dsn"`
	MetaDSN string `json:"meta_dsn,omitempty"`
}

type LoggingSection struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

type AuditSection struct {
	RetentionDays int `json:"retention_days"`
}

// MetricsSection configures persisted relay telemetry (per-minute buckets).
type MetricsSection struct {
	// RelayBucketRetentionDays drops relay_metric_buckets older than this many days (UTC minute buckets).
	RelayBucketRetentionDays int `json:"relay_bucket_retention_days"`
}

type RateLimitsSection struct {
	EventsPerMinutePerConnection int `json:"events_per_minute_per_connection"`
	BytesPerSecondPerConnection  int `json:"bytes_per_second_per_connection"`
	ReqsPerMinutePerConnection   int `json:"reqs_per_minute_per_connection"`
	MessagesPerMinutePerIP       int `json:"messages_per_minute_per_ip"`
}

type ConnectionLimitsSection struct {
	MaxOpen                       int  `json:"max_open"`
	// MaxOpenPerIP caps concurrent WebSockets per peer IP. Zero disables the cap.
	MaxOpenPerIP int `json:"max_open_per_ip"`
	MaxSubscriptionsPerConnection int  `json:"max_subscriptions_per_connection"`
	MaxFiltersPerReq              int  `json:"max_filters_per_req"`
	ConnectionsPerMinutePerIP     int  `json:"connections_per_minute_per_ip"`
	// IdleNoEventNoSubSeconds closes connections with no client EVENT and no open REQ
	// subscriptions after this many seconds. Zero disables the idle sweeper.
	IdleNoEventNoSubSeconds int  `json:"idle_no_event_no_sub_seconds"`
	ReadDeadlineSeconds     int  `json:"read_deadline_seconds"`
	WriteDeadlineSeconds    int  `json:"write_deadline_seconds"`
	DefaultQueryLimit       *int `json:"default_query_limit,omitempty"`
	QueryPageSize           *int `json:"query_page_size,omitempty"`
}

// DefaultQueryLimitIfUnset caps initial REQ results per filter when default_query_limit is omitted from JSON.
const DefaultQueryLimitIfUnset = 500

// DefaultQueryPageSizeIfUnset is the internal REQ read chunk size when query_page_size is omitted from JSON.
const DefaultQueryPageSizeIfUnset = 100

// EffectiveREQDefaultQueryLimit returns the cap applied when a subscription filter omits "limit".
// A nil config pointer uses DefaultQueryLimitIfUnset. A non-positive configured value disables that cap
// for omitted limits (the relay treats 0 as unlimited at apply time).
func EffectiveREQDefaultQueryLimit(p *int) int {
	if p == nil {
		return DefaultQueryLimitIfUnset
	}
	if *p <= 0 {
		return 0
	}
	return *p
}

// EffectiveQueryPageSize returns the internal REQ pagination chunk size.
// A nil config pointer uses DefaultQueryPageSizeIfUnset. A non-positive configured value disables paging
// (single query, legacy behavior).
func EffectiveQueryPageSize(p *int) int {
	if p == nil {
		return DefaultQueryPageSizeIfUnset
	}
	return *p
}

type WebSocketSection struct {
	CompressionEnabled bool `json:"compression_enabled"`
	MaxMessageBytes    int  `json:"max_message_bytes"`
}

type NIP11Section struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	PubKey             string `json:"pubkey"`
	Contact            string `json:"contact"`
	Software           string `json:"software"`
	CORSAllowAnyOrigin bool   `json:"cors_allow_any_origin"`
}

type NIPsSection struct {
	Enabled []int `json:"enabled"`
}

// NIP42Section configures NIP-42 client authentication (optional NIP).
type NIP42Section struct {
	RelayURL               string `json:"relay_url"`
	SendChallengeOnConnect bool   `json:"send_challenge_on_connect"`
	// CreatedAtSkewSeconds is the maximum allowed |now - event.created_at| for AUTH events (seconds).
	// Values <= 0 mean the relay uses its runtime default (600s).
	CreatedAtSkewSeconds      int      `json:"created_at_skew_seconds"`
	RequireAuthSubscribeKinds []int    `json:"require_auth_subscribe_kinds"`
	RequireAuthPublishKinds   []int    `json:"require_auth_publish_kinds"`
	AllowlistedPubkeys        []string `json:"allowlisted_pubkeys"`
}

// NIP17Section configures NIP-17 private direct messages (optional NIP).
type NIP17Section struct {
	// RejectGiftWrapWhenDisabled rejects incoming kind 1059 when NIP-17 is not in nips.enabled.
	// Omitted in JSON defaults to true. Ignored when NIP-17 is enabled.
	RejectGiftWrapWhenDisabled *bool `json:"reject_gift_wrap_when_disabled,omitempty"`
}

// NIP17RejectGiftWrapWhenDisabled reports whether kind 1059 publishes should be rejected while NIP-17 is off.
func NIP17RejectGiftWrapWhenDisabled(cfg *Config) bool {
	if cfg == nil {
		return true
	}
	for _, n := range cfg.NIPs.Enabled {
		if n == 17 {
			return false
		}
	}
	if cfg.NIP17.RejectGiftWrapWhenDisabled == nil {
		return true
	}
	return *cfg.NIP17.RejectGiftWrapWhenDisabled
}

// NIP29Section configures NIP-29 relay-based groups (optional NIP).
type NIP29Section struct {
	// LatePublicationMaxPastSeconds rejects events whose created_at is more than this many seconds in the past vs relay time. Zero means use the built-in default (86400).
	LatePublicationMaxPastSeconds int `json:"late_publication_max_past_seconds"`
	// StrictPreviousSameH requires each "previous" id prefix to resolve to an event whose "h" tag matches the publishing event's group id.
	StrictPreviousSameH bool `json:"strict_previous_same_h"`
}
