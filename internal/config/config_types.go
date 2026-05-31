package config

import "encoding/json"

// ConfigVersionCurrent is written to config_version after migration to the nips map model.
const ConfigVersionCurrent = 1

// Config matches the root object in config.example.json.
type Config struct {
	ConfigVersion           int                       `json:"config_version"`
	Relay                   RelaySection              `json:"relay"`
	Admin                   AdminSection              `json:"admin"`
	Database                DatabaseSection           `json:"database"`
	Logging                 LoggingSection            `json:"logging"`
	Audit                   AuditSection              `json:"audit"`
	Metrics                 MetricsSection            `json:"metrics"`
	RateLimits              RateLimitsSection         `json:"rate_limits"`
	ConnectionLimits        ConnectionLimitsSection   `json:"connection_limits"`
	WebSocket               WebSocketSection          `json:"websocket"`
	MaxSubscriptionIDLength int                       `json:"max_subscription_id_length"`
	NIP11                   NIP11Section              `json:"nip11"`
	NIP42                   NIP42Section              `json:"nip42"`
	NIPs                    map[string]NipPluginEntry `json:"nips"`
}

// NipPluginEntry holds enablement and plugin-owned settings for an optional NIP plugin.
type NipPluginEntry struct {
	Enabled  bool            `json:"enabled"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

type RelaySection struct {
	Port       int    `json:"port"`
	InstanceID string `json:"instance_id,omitempty"`
}

type AdminSection struct {
	Port int `json:"port"`
}

type DatabaseSection struct {
	Type string `json:"type"`
	DSN  string `json:"dsn"`
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
	MaxSubscriptionsPerConnection int  `json:"max_subscriptions_per_connection"`
	MaxFiltersPerReq              int  `json:"max_filters_per_req"`
	ConnectionsPerMinutePerIP     int  `json:"connections_per_minute_per_ip"`
	ReadDeadlineSeconds           int  `json:"read_deadline_seconds"`
	WriteDeadlineSeconds          int  `json:"write_deadline_seconds"`
	DefaultQueryLimit             *int `json:"default_query_limit,omitempty"`
}

// DefaultQueryLimitIfUnset caps initial REQ results per filter when default_query_limit is omitted from JSON.
const DefaultQueryLimitIfUnset = 500

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

// NIP42Section configures NIP-42 client authentication (optional core NIP).
type NIP42Section struct {
	Enabled                bool   `json:"enabled"`
	RelayURL               string `json:"relay_url"`
	SendChallengeOnConnect bool   `json:"send_challenge_on_connect"`
	// CreatedAtSkewSeconds is the maximum allowed |now - event.created_at| for AUTH events (seconds).
	// Values <= 0 mean the relay uses its runtime default (600s).
	CreatedAtSkewSeconds      int      `json:"created_at_skew_seconds"`
	RequireAuthSubscribeKinds []int    `json:"require_auth_subscribe_kinds"`
	RequireAuthPublishKinds   []int    `json:"require_auth_publish_kinds"`
	AllowlistedPubkeys        []string `json:"allowlisted_pubkeys"`
}
