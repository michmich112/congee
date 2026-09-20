package plugin

import (
	"context"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

type memStore struct {
	byID map[string]*nostr.Event
}

func (m *memStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	_ = ctx
	want := map[string]struct{}{}
	for _, f := range filters {
		for _, id := range f.IDs {
			want[id] = struct{}{}
		}
	}
	var out []*nostr.Event
	for id := range want {
		if ev, ok := m.byID[id]; ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

func TestGetEventsByIDsPreservesOrder(t *testing.T) {
	st := &memStore{byID: map[string]*nostr.Event{
		"aa": {ID: "aa", Content: "a"},
		"bb": {ID: "bb", Content: "b"},
		"cc": {ID: "cc", Content: "c"},
	}}
	got, err := GetEventsByIDs(context.Background(), st, []string{"cc", "missing", "aa", "cc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "cc" || got[1].ID != "aa" {
		t.Fatalf("got %#v", got)
	}
}
