package admin

import "testing"

func TestParseAuditLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want int
	}{
		{"", 50},
		{"50", 50},
		{"1000", 1000},
		{"5000", 5000},
		{"5001", maxAuditQueryLimit},
		{"999999", maxAuditQueryLimit},
		{"0", 50},
		{"-1", 50},
		{"nope", 50},
	}
	for _, tt := range tests {
		name := tt.raw
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := parseAuditLimit(tt.raw); got != tt.want {
				t.Fatalf("parseAuditLimit(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}
