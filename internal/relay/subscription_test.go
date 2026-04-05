package relay

import (
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
)

func TestSubscriptionManagerSubIDLength(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg)
	long := make([]byte, cfg.MaxSubscriptionIDLength+1)
	for i := range long {
		long[i] = 'a'
	}
	err := m.Add("c1", string(long), nil)
	if err == nil {
		t.Fatal("expected error for long sub id")
	}
	if err := m.Add("c1", "ok", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	if m.SubCount("c1") != 1 {
		t.Fatalf("sub count %d", m.SubCount("c1"))
	}
}

func TestSubscriptionManagerMaxSubs(t *testing.T) {
	cfg := minimalRelayCfg()
	cfg.ConnectionLimits.MaxSubscriptionsPerConnection = 1
	m := NewSubscriptionManager(cfg)
	m.RegisterSender("c1", func([]byte) bool { return true })
	if err := m.Add("c1", "a", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Add("c1", "b", nil); err != ErrTooManySubscriptions {
		t.Fatalf("got %v", err)
	}
}

func TestFiltersMatchOR(t *testing.T) {
	ev := &nostr.Event{Kind: 1, PubKey: "abc", Content: "x"}
	f1 := nostr.Filter{Kinds: []int{2}}
	f2 := nostr.Filter{Kinds: []int{1}}
	if filtersMatch([]nostr.Filter{f1, f2}, ev) != true {
		t.Fatal("expected OR match")
	}
}

func minimalRelayCfg() *config.Config {
	return &config.Config{
		MaxSubscriptionIDLength: 64,
		ConnectionLimits: config.ConnectionLimitsSection{
			MaxSubscriptionsPerConnection: 10,
			MaxFiltersPerReq:              5,
			ConnectionsPerMinutePerIP:     100,
		},
	}
}
