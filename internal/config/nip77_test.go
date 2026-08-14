package config_test

import (
	"encoding/json"
	"testing"

	"github.com/michmich112/congee/internal/config"
)

func TestValidateNIP77Upstream(t *testing.T) {
	c := config.DefaultConfig()
	c.NIPs.Enabled = append(c.NIPs.Enabled, 77)
	c.NIP77.Upstreams = []config.NIP77Upstream{
		{Name: "a", URL: "wss://relay.example/", Filters: []json.RawMessage{json.RawMessage(`{"kinds":[1]}`)}, IntervalSeconds: 3600, Enabled: true},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNIP77Enabled(t *testing.T) {
	c := config.DefaultConfig()
	if config.NIP77Enabled(c) {
		t.Fatal("expected false")
	}
	c.NIPs.Enabled = append(c.NIPs.Enabled, 77)
	if !config.NIP77Enabled(c) {
		t.Fatal("expected true")
	}
}
