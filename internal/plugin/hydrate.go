package plugin

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

type eventQuerier interface {
	QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error)
}

// GetEventsByIDs loads events and returns them in the same order as ids. Missing ids are skipped.
func GetEventsByIDs(ctx context.Context, store eventQuerier, ids []string) ([]*nostr.Event, error) {
	if store == nil || len(ids) == 0 {
		return nil, nil
	}
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil, nil
	}
	events, err := store.QueryEvents(ctx, []nostr.Filter{{IDs: unique}})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*nostr.Event, len(events))
	for _, ev := range events {
		if ev != nil {
			byID[ev.ID] = ev
		}
	}
	out := make([]*nostr.Event, 0, len(ids))
	got := make(map[string]struct{})
	for _, id := range ids {
		ev, ok := byID[id]
		if !ok {
			continue
		}
		if _, dup := got[id]; dup {
			continue
		}
		got[id] = struct{}{}
		out = append(out, ev)
	}
	return out, nil
}
