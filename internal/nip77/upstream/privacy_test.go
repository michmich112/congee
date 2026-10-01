package upstream

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUpstreamSyncAcceptsConfiguredRecoveryFilters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		filter  string
		blocked bool
	}{
		{"wildcard", `{}`, false},
		{"ids only", `{"ids":["` + strings.Repeat("a", 64) + `"]}`, false},
		{"kind 1059", `{"kinds":[1059]}`, false},
		{"kind 21059", `{"kinds":[21059]}`, false},
		{"mixed kinds", `{"kinds":[30402,1059]}`, false},
		{"search unsupported", `{"search":"x"}`, true},
		{"products", `{"kinds":[30402]}`, false},
		{"merchant metadata", `{"kinds":[0,10002]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseUpstreamFilters([]json.RawMessage{json.RawMessage(tt.filter)})
			if tt.blocked && err == nil {
				t.Fatal("unsafe upstream filter accepted")
			}
			if !tt.blocked && err != nil {
				t.Fatalf("safe upstream filter rejected: %v", err)
			}
		})
	}
}
