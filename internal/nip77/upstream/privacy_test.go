package upstream

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUpstreamSyncFiltersDoNotDiscloseGiftWrapIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		filter  string
		blocked bool
	}{
		{"wildcard", `{}`, true},
		{"ids only", `{"ids":["` + strings.Repeat("a", 64) + `"]}`, true},
		{"kind 1059", `{"kinds":[1059]}`, true},
		{"kind 21059", `{"kinds":[21059]}`, true},
		{"mixed kinds", `{"kinds":[30402,1059]}`, true},
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
