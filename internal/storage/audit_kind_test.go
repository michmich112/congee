package storage_test

import (
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/storage"
)

func TestParseAuditDetailTrailingKind(t *testing.T) {
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	detail := "event_id=" + id + " conn_id=abcd1234 stored=true kind=42"
	k, ok := storage.ParseAuditDetailTrailingKind(detail)
	if !ok || k != 42 {
		t.Fatalf("want 42 ok=true, got %d ok=%v", k, ok)
	}
	if _, ok := storage.ParseAuditDetailTrailingKind("no kind here"); ok {
		t.Fatal("expected false")
	}
	if _, ok := storage.ParseAuditDetailTrailingKind("kind=1"); ok {
		t.Fatal("missing leading space should not match")
	}
}

func TestDedupeSortNonNegInts(t *testing.T) {
	got := storage.DedupeSortNonNegInts([]int{3, 1, 3, -1, 2})
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if storage.DedupeSortNonNegInts(nil) != nil {
		t.Fatalf("nil in should give nil out")
	}
	if len(storage.DedupeSortNonNegInts([]int{})) != 0 {
		t.Fatal("empty in should give empty out")
	}
}

func TestAuditDetailKindSuffixMatchOr(t *testing.T) {
	sql, args := storage.AuditDetailKindSuffixMatchOr(true, []int{1, 10})
	if sql == "" || len(args) != 4 {
		t.Fatalf("sqlite OR: sql=%q args=%v", sql, args)
	}
	sqlPg, argsPg := storage.AuditDetailKindSuffixMatchOr(false, []int{2})
	if !strings.Contains(sqlPg, "right(detail") || len(argsPg) != 2 {
		t.Fatalf("postgres: sql=%q args=%v", sqlPg, argsPg)
	}
	s, a := storage.AuditDetailKindSuffixMatchOr(true, nil)
	if s != "" || a != nil {
		t.Fatalf("nil kinds: %q %v", s, a)
	}
}
