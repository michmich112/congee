package relay

import (
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

func TestSubscriptionManagerTotalSubscriptions(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	m := NewSubscriptionManager(cfg, zerolog.Nop())
	if n := m.TotalSubscriptions(); n != 0 {
		t.Fatalf("empty want 0 got %d", n)
	}
	_ = m.Add("c1", "a", []nostr.Filter{{Kinds: []int{1}}})
	_ = m.Add("c1", "b", []nostr.Filter{{Kinds: []int{2}}})
	_ = m.Add("c2", "x", []nostr.Filter{{Kinds: []int{3}}})
	if n := m.TotalSubscriptions(); n != 3 {
		t.Fatalf("want 3 got %d", n)
	}
	m.Remove("c1", "a")
	if n := m.TotalSubscriptions(); n != 2 {
		t.Fatalf("want 2 got %d", n)
	}
}
