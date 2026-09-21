package plugin

import (
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	sdk "github.com/michmich112/congee/sdk/plugin"
)

func TestMatchOnStoredEventKinds(t *testing.T) {
	subs := []sdk.TrafficSubscription{{OnStoredEvent: true, Kinds: []int{30402, 5}}}
	if !MatchOnStoredEvent(subs, 30402) {
		t.Fatal("expected match")
	}
	if MatchOnStoredEvent(subs, 1) {
		t.Fatal("kind 1 should not match")
	}
}

func TestMatchOnStoredEventEmptyKinds(t *testing.T) {
	subs := []sdk.TrafficSubscription{{OnStoredEvent: true}}
	if MatchOnStoredEvent(subs, 30402) {
		t.Fatal("empty kinds must not match")
	}
}

func TestMatchInterceptREQSearch(t *testing.T) {
	subs := []sdk.TrafficSubscription{{
		MessageTypes: []string{"REQ"},
		ReqHasSearch: true,
		InterceptREQ: true,
	}}
	s := "shoes"
	filters := []nostr.Filter{{Search: &s}}
	if !MatchInterceptREQ(subs, filters) {
		t.Fatal("search should match")
	}
	if MatchInterceptREQ(subs, []nostr.Filter{{Kinds: []int{1}}}) {
		t.Fatal("kind 1 only should not match search-only sub")
	}
}

func TestMatchInterceptREQNoKindsEmptyREQ(t *testing.T) {
	subs := []sdk.TrafficSubscription{{
		MessageTypes: []string{"REQ"},
		Kinds:        []int{30402},
		InterceptREQ: true,
	}}
	if MatchInterceptREQ(subs, []nostr.Filter{{}}) {
		t.Fatal("empty kinds REQ should not match kinds-only subscription")
	}
	if !MatchInterceptREQ(subs, []nostr.Filter{{Kinds: []int{30402}}}) {
		t.Fatal("product kinds should match")
	}
}

func TestMatchObserveEvent(t *testing.T) {
	subs := []sdk.TrafficSubscription{{
		MessageTypes: []string{"EVENT"},
		Kinds:        []int{1},
		Observe:      true,
	}}
	if !MatchObserve(subs, &nostr.EventMessage{Event: nostr.Event{Kind: 1}}) {
		t.Fatal("expected observe")
	}
	if MatchObserve(subs, &nostr.EventMessage{Event: nostr.Event{Kind: 7}}) {
		t.Fatal("kind 7 should not observe")
	}
}
