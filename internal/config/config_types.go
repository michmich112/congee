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
}

type ConnectionLimitsSection struct {
	MaxOpen                       int `json:"max_open"`
	MaxSubscriptionsPerConnection int `json:"max_subscriptions_per_connection"`
	ReadDeadlineSeconds           int `json:"read_deadline_seconds"`
	WriteDeadlineSeconds          int `json:"write_deadline_seconds"`
}

type WebSocketSection struct {
	CompressionEnabled bool `json:"compression_enabled"`
}

type NIP11Section struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	PubKey        string `json:"pubkey"`
	Contact       string `json:"contact"`
	SupportedNIPs []int  `json:"supported_nips"`
	Software      string `json:"software"`
	Version       string `json:"version"`
}

type NIPsSection struct {
	Enabled []int `json:"enabled"`
}
