package storage

import (
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

func TestCountFilterSubQuery_nilSkipped(t *testing.T) {
	_, _, skip := CountFilterSubQuery(nil)
	if !skip {
		t.Fatal("expected nil filter to be skipped")
	}
}

func TestCountFilterSubQuery_searchSkipped(t *testing.T) {
	s := "hello"
	f := nostr.Filter{Search: &s}
	_, _, skip := CountFilterSubQuery(&f)
	if !skip {
		t.Fatal("expected search filter to be skipped")
	}
}

func TestCountFilterSubQuery_emptyMatchesAllIDs(t *testing.T) {
	sql, args, skip := CountFilterSubQuery(&nostr.Filter{})
	if skip {
		t.Fatal("unexpected skip")
	}
	if sql != "SELECT id FROM events" {
		t.Fatalf("sql: got %q", sql)
	}
	if len(args) != 0 {
		t.Fatalf("args: got %v", args)
	}
}

func TestCountFilterSubQuery_kinds(t *testing.T) {
	sql, args, skip := CountFilterSubQuery(&nostr.Filter{Kinds: []int{1, 7}})
	if skip {
		t.Fatal("unexpected skip")
	}
	if want := "SELECT id FROM events WHERE kind IN (?, ?)"; sql != want {
		t.Fatalf("sql: got %q want %q", sql, want)
	}
	if len(args) != 2 || args[0] != 1 || args[1] != 7 {
		t.Fatalf("args: got %v", args)
	}
}
