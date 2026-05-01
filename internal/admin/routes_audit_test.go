package admin

import (
	"net/http/httptest"
	"testing"
)

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
		{"5001", 5001},
		{"999999", 999999},
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

func TestParseAuditKindsQuery(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/x?kind=1&kind=2&kind=1", nil)
	got := parseAuditKindsQuery(r)
	want := []int{1, 2}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	r2 := httptest.NewRequest("GET", "/x?kind=3%2C4%2C5", nil)
	got2 := parseAuditKindsQuery(r2)
	want2 := []int{3, 4, 5}
	if len(got2) != len(want2) {
		t.Fatalf("comma: got %v want %v", got2, want2)
	}
	if parseAuditKindsQuery(httptest.NewRequest("GET", "/x", nil)) != nil {
		t.Fatal("empty query should yield nil kinds slice")
	}
	r3 := httptest.NewRequest("GET", "/x?kind=-1&kind=2", nil)
	if g := parseAuditKindsQuery(r3); len(g) != 1 || g[0] != 2 {
		t.Fatalf("negative ignored: got %v want [2]", g)
	}
}
