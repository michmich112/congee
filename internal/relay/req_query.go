package relay

import (
	"context"
	"sort"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

// applyDefaultQueryLimit returns a copy of filters with filter.Limit set to the given default
// when it is nil. Returns the original slice unchanged when defaultLimit <= 0 or no filter
// needs the default applied. A default value <= 0 means unlimited (no limit is applied).
func applyDefaultQueryLimit(filters []nostr.Filter, defaultLimit int) []nostr.Filter {
	if defaultLimit <= 0 {
		return filters
	}
	for i := range filters {
		if filters[i].Limit == nil {
			result := make([]nostr.Filter, len(filters))
			copy(result, filters)
			for j := range result {
				if result[j].Limit == nil {
					lim := defaultLimit
					result[j].Limit = &lim
				}
			}
			return result
		}
	}
	return filters
}

// queryInitialREQEvents loads the initial snapshot for a REQ (OR across filters).
// Filters with NIP-50 search use SearchEvents; others use QueryEvents. When searchEnabled
// is false, callers must reject REQ earlier if any filter HasSearch.
func queryInitialREQEvents(ctx context.Context, store storage.Store, filters []nostr.Filter, searchEnabled bool, defaultQueryLimit int) ([]*nostr.Event, error) {
	filters = applyDefaultQueryLimit(filters, defaultQueryLimit)
	if len(filters) == 0 {
		return nil, nil
	}
	byID := make(map[string]*nostr.Event)
	for i := range filters {
		f := &filters[i]
		var evs []*nostr.Event
		var err error
		if f.HasSearch() {
			if !searchEnabled {
				continue
			}
			evs, err = store.SearchEvents(ctx, f.SearchText(), f.WithoutSearch())
		} else {
			evs, err = store.QueryEvents(ctx, []nostr.Filter{filters[i]})
		}
		if err != nil {
			return nil, err
		}
		for _, ev := range evs {
			byID[ev.ID] = ev
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := byID[ids[i]], byID[ids[j]]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		return a.ID < b.ID
	})
	out := make([]*nostr.Event, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out, nil
}
