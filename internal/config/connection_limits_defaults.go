package config

import "encoding/json"

// applyUnsetConnectionLimitsDefaults fills max_open_per_ip and
// idle_no_event_no_sub_seconds when those keys are absent from JSON.
// Explicit zero values (unlimited / disabled) are preserved.
func applyUnsetConnectionLimitsDefaults(c *Config, data []byte) {
	if c == nil {
		return
	}
	def := DefaultConfig().ConnectionLimits
	keys, sectionPresent := connectionLimitsKeysPresent(data)
	if !sectionPresent {
		c.ConnectionLimits.MaxOpenPerIP = def.MaxOpenPerIP
		c.ConnectionLimits.IdleNoEventNoSubSeconds = def.IdleNoEventNoSubSeconds
		return
	}
	if _, ok := keys["max_open_per_ip"]; !ok {
		c.ConnectionLimits.MaxOpenPerIP = def.MaxOpenPerIP
	}
	if _, ok := keys["idle_no_event_no_sub_seconds"]; !ok {
		c.ConnectionLimits.IdleNoEventNoSubSeconds = def.IdleNoEventNoSubSeconds
	}
}

func connectionLimitsKeysPresent(data []byte) (keys map[string]struct{}, sectionPresent bool) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false
	}
	clRaw, ok := root["connection_limits"]
	if !ok {
		return nil, false
	}
	var cl map[string]json.RawMessage
	if err := json.Unmarshal(clRaw, &cl); err != nil {
		return nil, true
	}
	keys = make(map[string]struct{}, len(cl))
	for k := range cl {
		keys[k] = struct{}{}
	}
	return keys, true
}

func unmarshalConfigJSON(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	applyUnsetConnectionLimitsDefaults(&c, data)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
