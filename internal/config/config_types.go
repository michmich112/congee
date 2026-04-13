package config

// Config matches the root object in config.example.json.
type Config struct {
	Relay                   RelaySection            `json:"relay"`
	Admin                   AdminSection            `json:"admin"`
	Database                DatabaseSection         `json:"database"`
	Logging                 LoggingSection          `json:"logging"`
	Audit                   AuditSection            `json:"audit"`
	RateLimits              RateLimitsSection       `json:"rate_limits"`
	ConnectionLimits        ConnectionLimitsSection `json:"connection_limits"`
	WebSocket               WebSocketSection        `json:"websocket"`
	MaxSubscriptionIDLength int                     `json:"max_subscription_id_length"`
	NIP11                   NIP11Section            `json:"nip11"`
	NIP42                   NIP42Section            `json:"nip42"`
	NIPs                    NIPsSection             `json:"nips"`
}

type RelaySection struct {
	Port int `json:"port"`
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

type RateLimitsSection struct {
	EventsPerMinutePerConnection int `json:"events_per_minute_per_connection"`
	BytesPerSecondPerConnection  int `json:"bytes_per_second_per_connection"`
	ReqsPerMinutePerConnection   int `json:"reqs_per_minute_per_connection"`
	MessagesPerMinutePerIP       int `json:"messages_per_minute_per_ip"`
}

type ConnectionLimitsSection struct {
	MaxOpen                       int `json:"max_open"`
	MaxSubscriptionsPerConnection int `json:"max_subscriptions_per_connection"`
	MaxFiltersPerReq              int `json:"max_filters_per_req"`
	ConnectionsPerMinutePerIP     int `json:"connections_per_minute_per_ip"`
	ReadDeadlineSeconds           int `json:"read_deadline_seconds"`
	WriteDeadlineSeconds          int `json:"write_deadline_seconds"`
}

type WebSocketSection struct {
	CompressionEnabled bool `json:"compression_enabled"`
	MaxMessageBytes    int `json:"max_message_bytes"`
}

type NIP11Section struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	PubKey             string `json:"pubkey"`
	Contact            string `json:"contact"`
	Software           string `json:"software"`
	CORSAllowAnyOrigin bool `json:"cors_allow_any_origin"`
}

type NIPsSection struct {
	Enabled []int `json:"enabled"`
}

// NIP42Section configures NIP-42 client authentication (optional NIP).
type NIP42Section struct {
	RelayURL                  string   `json:"relay_url"`
	SendChallengeOnConnect    bool     `json:"send_challenge_on_connect"`
	CreatedAtSkewSeconds      int      `json:"created_at_skew_seconds"`
	RequireAuthSubscribeKinds []int    `json:"require_auth_subscribe_kinds"`
	RequireAuthPublishKinds   []int    `json:"require_auth_publish_kinds"`
	AllowlistedPubkeys        []string `json:"allowlisted_pubkeys"`
}
