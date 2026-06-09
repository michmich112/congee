package relay

import (
	"context"
	"sort"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

// applyDefaultQueryLimit returns a copy of filters with filter.Limit set to defaultLimit only
// when the client omitted "limit" (nil). Filters with an explicit non-positive limit are left
// unchanged so storage treats them as unlimited. Returns the original slice unchanged when
// defaultLimit <= 0 or no filter has a nil limit.
func applyDefaultQueryLimit(filters []nostr.Filter, defaultLimit int) []nostr.Filter {
	if defaultLimit <= 0 {
		return filters
	}
	needsCopy := false
	for i := range filters {
		if filters[i].Limit == nil {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return filters
	}
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

type filterCursor struct {
	base      nostr.Filter
	until     *int64 // nil on first page; then oldest_created_at-1
	remaining int    // budget; <=0 means unlimited (default_query_limit=0 or client limit<=0)
	exhausted bool
}

type reqQueryState struct {
	nonSearch  []filterCursor
	search     []nostr.Filter
	seen       map[string]struct{}
	searchDone bool
}

func newREQQueryState(filters []nostr.Filter, defaultQueryLimit int, searchEnabled bool) *reqQueryState {
	filters = applyDefaultQueryLimit(filters, defaultQueryLimit)
	st := &reqQueryState{
		seen: make(map[string]struct{}),
	}
	for i := range filters {
		f := filters[i]
		if f.HasSearch() {
			if searchEnabled {
				st.search = append(st.search, f)
			}
			continue
		}
		remaining := 0
		if f.Limit != nil && *f.Limit > 0 {
			remaining = *f.Limit
		}
		st.nonSearch = append(st.nonSearch, filterCursor{
			base:      f,
			remaining: remaining,
		})
	}
	return st
}

func mergePageEvents(pageByID map[string]*nostr.Event, seen map[string]struct{}) []*nostr.Event {
	ids := make([]string, 0, len(pageByID))
	for id := range pageByID {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := pageByID[ids[i]], pageByID[ids[j]]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		return a.ID < b.ID
	})
	out := make([]*nostr.Event, 0, len(ids))
	for _, id := range ids {
		out = append(out, pageByID[id])
	}
	return out
}

func fetchREQAll(ctx context.Context, store storage.Store, st *reqQueryState) ([]*nostr.Event, bool, error) {
	pageByID := make(map[string]*nostr.Event)
	for i := range st.search {
		f := &st.search[i]
		evs, err := store.SearchEvents(ctx, f.SearchText(), f.WithoutSearch())
		if err != nil {
			return nil, false, err
		}
		for _, ev := range evs {
			pageByID[ev.ID] = ev
		}
	}
	st.searchDone = true
	for i := range st.nonSearch {
		fc := &st.nonSearch[i]
		evs, err := store.QueryEvents(ctx, []nostr.Filter{fc.base})
		if err != nil {
			return nil, false, err
		}
		for _, ev := range evs {
			pageByID[ev.ID] = ev
		}
		fc.exhausted = true
	}
	return mergePageEvents(pageByID, st.seen), false, nil
}

// fetchREQPage loads one page of initial REQ snapshot events (OR across filters).
// When pageSize <= 0, all matching events are returned in a single page (legacy behavior).
// Search filters run once on the first page only; non-search filters paginate via until cursors.
func fetchREQPage(ctx context.Context, store storage.Store, st *reqQueryState, pageSize int) ([]*nostr.Event, bool, error) {
	if pageSize <= 0 {
		return fetchREQAll(ctx, store, st)
	}
	if len(st.nonSearch) == 0 && len(st.search) == 0 {
		return nil, false, nil
	}

	pageByID := make(map[string]*nostr.Event)

	if !st.searchDone {
		for i := range st.search {
			f := &st.search[i]
			evs, err := store.SearchEvents(ctx, f.SearchText(), f.WithoutSearch())
			if err != nil {
				return nil, false, err
			}
			for _, ev := range evs {
				pageByID[ev.ID] = ev
			}
		}
		st.searchDone = true
	}

	for i := range st.nonSearch {
		fc := &st.nonSearch[i]
		if fc.exhausted {
			continue
		}
		lim := pageSize
		if fc.remaining > 0 && fc.remaining < lim {
			lim = fc.remaining
		}
		queryF := fc.base
		queryF.Limit = &lim
		if fc.until != nil {
			queryF.Until = fc.until
		}
		evs, err := store.QueryEvents(ctx, []nostr.Filter{queryF})
		if err != nil {
			return nil, false, err
		}
		for _, ev := range evs {
			pageByID[ev.ID] = ev
		}
		if len(evs) > 0 {
			oldest := evs[len(evs)-1].CreatedAt
			u := oldest - 1
			fc.until = &u
		}
		if fc.remaining > 0 {
			fc.remaining -= len(evs)
			if fc.remaining <= 0 {
				fc.exhausted = true
			}
		}
		if len(evs) < lim {
			fc.exhausted = true
		}
	}

	hasMore := false
	for i := range st.nonSearch {
		if !st.nonSearch[i].exhausted {
			hasMore = true
			break
		}
	}
	return mergePageEvents(pageByID, st.seen), hasMore, nil
}

// queryInitialREQEvents loads the initial snapshot for a REQ (OR across filters).
// Filters with NIP-50 search use SearchEvents; others use QueryEvents. When searchEnabled
// is false, callers must reject REQ earlier if any filter HasSearch. defaultQueryLimit is
// the effective cap from config (see config.EffectiveREQDefaultQueryLimit); values <= 0
// mean no default is applied to filters with a nil limit.
func queryInitialREQEvents(ctx context.Context, store storage.Store, filters []nostr.Filter, searchEnabled bool, defaultQueryLimit int) ([]*nostr.Event, error) {
	st := newREQQueryState(filters, defaultQueryLimit, searchEnabled)
	evs, _, err := fetchREQPage(ctx, store, st, 0)
	return evs, err
}
