package registry

import (
	"slices"
	"sort"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

type routeEntry struct {
	pluginID string
	priority int
	order    int
	route    plugin.Route
	manifest plugin.Manifest
}

type routingIndex struct {
	entries []routeEntry
}

func newRoutingIndex(entries []routeEntry) *routingIndex {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].priority != entries[j].priority {
			return entries[i].priority < entries[j].priority
		}
		return entries[i].order < entries[j].order
	})
	return &routingIndex{entries: entries}
}

func (idx *routingIndex) matchEvent(ev *nostr.Event) []routeEntry {
	if ev == nil {
		return nil
	}
	var out []routeEntry
	for _, e := range idx.entries {
		if e.route.Event == nil {
			continue
		}
		if routeMatchesEvent(e.route, ev) {
			out = append(out, e)
		}
	}
	return out
}

func (idx *routingIndex) matchFilter(f *nostr.Filter) []routeEntry {
	if f == nil {
		return nil
	}
	var out []routeEntry
	for _, e := range idx.entries {
		if e.route.Req == nil {
			continue
		}
		if routeMatchesFilter(e.route, f) {
			out = append(out, e)
		}
	}
	return out
}

func routeMatchesEvent(r plugin.Route, ev *nostr.Event) bool {
	if r.CatchAll {
		return true
	}
	if r.TagMatch != "" && eventHasTag(ev, string(r.TagMatch)) {
		return true
	}
	if len(r.Kinds) > 0 && slices.Contains(r.Kinds, ev.Kind) {
		return true
	}
	return false
}

func routeMatchesFilter(r plugin.Route, f *nostr.Filter) bool {
	if r.CatchAll {
		return true
	}
	if r.TagMatch != "" {
		key := "#" + string(r.TagMatch)
		if vals, ok := f.Tag[key]; ok && len(vals) > 0 {
			return true
		}
	}
	if len(r.Kinds) > 0 {
		if len(f.Kinds) == 0 {
			return true
		}
		for _, k := range f.Kinds {
			if slices.Contains(r.Kinds, k) {
				return true
			}
		}
	}
	return false
}

func eventHasTag(ev *nostr.Event, letter string) bool {
	for _, t := range ev.Tags {
		if len(t) > 0 && t[0] == letter {
			return true
		}
	}
	return false
}

func eventRequiresAuth(matches []routeEntry, ec *plugin.EventContext) bool {
	if ec == nil || ec.Conn == nil {
		return false
	}
	for _, m := range matches {
		if m.route.Event != nil && m.route.Event.RequiresAuth && !ec.Conn.HasAuth() {
			return true
		}
	}
	return false
}

func reqRequiresAuth(matches []routeEntry, conn plugin.ConnInfo) bool {
	if conn == nil {
		return false
	}
	if conn.HasAuth() {
		return false
	}
	for _, m := range matches {
		if m.route.Req != nil && m.route.Req.RequiresAuth {
			return true
		}
	}
	return false
}

func reqRequiresAuthForFilters(idx *routingIndex, conn plugin.ConnInfo, filters []nostr.Filter) bool {
	if conn != nil && conn.HasAuth() {
		return false
	}
	for i := range filters {
		matches := idx.matchFilter(&filters[i])
		if reqRequiresAuth(matches, conn) {
			return true
		}
	}
	return false
}
