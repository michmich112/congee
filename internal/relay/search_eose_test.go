package relay

import (
	"context"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

// searchStoreStub returns one matching event from SearchEvents.
type searchStoreStub struct {
	visibilityStoreStub
	ev *nostr.Event
}

func (s *searchStoreStub) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	_ = ctx
	if s.ev != nil {
		return []*nostr.Event{s.ev}, nil
	}
	return nil, nil
}

func (s *searchStoreStub) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	_ = ctx
	if s.ev != nil {
		return []*nostr.Event{s.ev}, nil
	}
	return nil, nil
}

// TestSearchREQNoMatchesSendsEOSEOnly verifies that a NIP-50 search REQ whose
// query matches no stored events sends EOSE and zero EVENT frames.
func TestSearchREQNoMatchesSendsEOSEOnly(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{} // SearchEvents returns nil, nil -> no matches
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	RegisterNIP50(srv, st)

	c := registerTestConnLargeSend(t, srv, "search-nomatch")
	q := "zzz-no-such-event"
	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{Search: &q}},
	}
	if err := handleREQ(context.Background(), srv, c, req, true); err != nil {
		t.Fatal(err)
	}
	types := drainOutboundChan(t, c, 4)
	if len(types) != 1 || types[0] != "EOSE" {
		t.Fatalf("want only EOSE (search matched nothing), got %#v", types)
	}
}

// TestSearchREQMatchesSendsEventsThenEOSE verifies a matching search sends EVENT
// frames and finishes with EOSE, so a client sees events before the terminator.
func TestSearchREQMatchesSendsEventsThenEOSE(t *testing.T) {
	t.Parallel()
	ev := &nostr.Event{
		ID:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Kind:    1,
		Content: "alpha beta",
	}
	st := &searchStoreStub{ev: ev}
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	RegisterNIP50(srv, st)

	c := registerTestConnLargeSend(t, srv, "search-match")
	q := "alpha"
	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{Search: &q}},
	}
	if err := handleREQ(context.Background(), srv, c, req, true); err != nil {
		t.Fatal(err)
	}
	types := drainOutboundChan(t, c, 8)
	if len(types) < 2 {
		t.Fatalf("want EVENT then EOSE, got %#v", types)
	}
	if types[0] != "EVENT" {
		t.Fatalf("want first frame EVENT, got %#v", types)
	}
	if types[len(types)-1] != "EOSE" {
		t.Fatalf("want final frame EOSE, got %#v", types)
	}
}
