package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// NIP77Enabled reports whether NIP-77 is listed in nips.enabled.
func NIP77Enabled(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	for _, n := range cfg.NIPs.Enabled {
		if n == 77 {
			return true
		}
	}
	return false
}

func effectiveNIP77Int(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// EffectiveNIP77MaxRecordsPerQuery returns 0 for unlimited when config is 0 explicitly after normalization.
func EffectiveNIP77MaxRecordsPerQuery(cfg *Config) int {
	if cfg == nil {
		return DefaultNIP77MaxRecordsPerQuery
	}
	return cfg.NIP77.MaxRecordsPerQuery
}

func EffectiveNIP77SessionIdleTimeout(cfg *Config) int {
	return effectiveNIP77Int(cfgValue(cfg).NIP77.SessionIdleTimeoutSeconds, DefaultNIP77SessionIdleTimeoutSeconds)
}

func EffectiveNIP77FrameSizeLimit(cfg *Config) int {
	return effectiveNIP77Int(cfgValue(cfg).NIP77.FrameSizeLimitBytes, DefaultNIP77FrameSizeLimitBytes)
}

func EffectiveNIP77MaxConcurrentSessions(cfg *Config) int {
	return effectiveNIP77Int(cfgValue(cfg).NIP77.MaxConcurrentSessions, DefaultNIP77MaxConcurrentSessions)
}

func EffectiveNIP77MaxConcurrentLoads(cfg *Config) int {
	return effectiveNIP77Int(cfgValue(cfg).NIP77.MaxConcurrentLoads, DefaultNIP77MaxConcurrentLoads)
}

func EffectiveNIP77NegOpenPerMinute(cfg *Config) int {
	return effectiveNIP77Int(cfgValue(cfg).NIP77.NegOpenPerMinutePerConnection, DefaultNIP77NegOpenPerMinutePerConnection)
}

func EffectiveNIP77NegMsgPerMinute(cfg *Config) int {
	return effectiveNIP77Int(cfgValue(cfg).NIP77.NegMsgPerMinutePerConnection, DefaultNIP77NegMsgPerMinutePerConnection)
}

func EffectiveNIP77BackpressureReqQueueDepth(cfg *Config) int {
	if cfg == nil {
		return DefaultNIP77BackpressureReqQueueDepth
	}
	return cfg.NIP77.BackpressureReqQueueDepth
}

func cfgValue(cfg *Config) Config {
	if cfg == nil {
		return Config{}
	}
	return *cfg
}

func validateNIP77(cfg *Config) error {
	if cfg == nil || !NIP77Enabled(cfg) {
		return nil
	}
	n := cfg.NIP77
	if n.MaxRecordsPerQuery < 0 {
		return errors.New("config: nip77.max_records_per_query must be >= 0")
	}
	if n.SessionIdleTimeoutSeconds < 0 {
		return errors.New("config: nip77.session_idle_timeout_seconds must be >= 0")
	}
	if n.FrameSizeLimitBytes < 0 {
		return errors.New("config: nip77.frame_size_limit_bytes must be >= 0")
	}
	if n.MaxConcurrentSessions < 0 {
		return errors.New("config: nip77.max_concurrent_sessions must be >= 0")
	}
	if n.MaxConcurrentLoads < 0 {
		return errors.New("config: nip77.max_concurrent_loads must be >= 0")
	}
	if n.NegOpenPerMinutePerConnection < 0 {
		return errors.New("config: nip77.neg_open_per_minute_per_connection must be >= 0")
	}
	if n.NegMsgPerMinutePerConnection < 0 {
		return errors.New("config: nip77.neg_msg_per_minute_per_connection must be >= 0")
	}
	if n.BackpressureReqQueueDepth < 0 {
		return errors.New("config: nip77.backpressure_req_queue_depth must be >= 0")
	}
	names := make(map[string]struct{})
	for i, u := range n.Upstreams {
		name := strings.TrimSpace(u.Name)
		if name == "" {
			return fmt.Errorf("config: nip77.upstreams[%d].name is required", i)
		}
		if _, dup := names[name]; dup {
			return fmt.Errorf("config: nip77.upstreams[%d]: duplicate name %q", i, name)
		}
		names[name] = struct{}{}
		rawURL := strings.TrimSpace(u.URL)
		if rawURL == "" {
			return fmt.Errorf("config: nip77.upstreams[%d].url is required", i)
		}
		pu, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("config: nip77.upstreams[%d].url: %w", i, err)
		}
		if pu.Scheme != "ws" && pu.Scheme != "wss" {
			return fmt.Errorf("config: nip77.upstreams[%d].url must use ws or wss", i)
		}
		if len(u.Filters) == 0 {
			return fmt.Errorf("config: nip77.upstreams[%d].filters must be non-empty", i)
		}
		if u.IntervalSeconds > 0 && u.IntervalSeconds < 60 {
			return fmt.Errorf("config: nip77.upstreams[%d].interval_seconds must be >= 60 when set", i)
		}
	}
	return nil
}
