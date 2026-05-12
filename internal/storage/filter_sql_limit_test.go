package storage

import (
	"math"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

func TestFilterSQLLimit(t *testing.T) {
	t.Run("applyLimits false", func(t *testing.T) {
		lim := 5
		if g := FilterSQLLimit(&nostr.Filter{Limit: &lim}, false); g != nil {
			t.Fatalf("got %v want nil", g)
		}
	})
	t.Run("nil filter", func(t *testing.T) {
		if g := FilterSQLLimit(nil, true); g != nil {
			t.Fatalf("got %v want nil", g)
		}
	})
	t.Run("nil limit", func(t *testing.T) {
		if g := FilterSQLLimit(&nostr.Filter{Kinds: []int{1}}, true); g != nil {
			t.Fatalf("got %v want nil", g)
		}
	})
	t.Run("zero limit", func(t *testing.T) {
		z := 0
		if g := FilterSQLLimit(&nostr.Filter{Limit: &z}, true); g != nil {
			t.Fatalf("got %v want nil", g)
		}
	})
	t.Run("negative limit", func(t *testing.T) {
		n := -1
		if g := FilterSQLLimit(&nostr.Filter{Limit: &n}, true); g != nil {
			t.Fatalf("got %v want nil", g)
		}
	})
	t.Run("positive limit", func(t *testing.T) {
		v := 42
		g := FilterSQLLimit(&nostr.Filter{Limit: &v}, true)
		if g == nil || *g != 42 {
			t.Fatalf("got %v want 42", g)
		}
	})
	t.Run("max int32 limit is preserved", func(t *testing.T) {
		v := math.MaxInt32
		g := FilterSQLLimit(&nostr.Filter{Limit: &v}, true)
		if g == nil || *g != math.MaxInt32 {
			t.Fatalf("got %v want MaxInt32", g)
		}
	})
}
