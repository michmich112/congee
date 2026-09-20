package plugin

import (
	"strings"

	"github.com/michmich112/congee/internal/nostr"
	sdk "github.com/michmich112/congee/sdk/plugin"
)

func kindIn(kinds []int, kind int) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func kindsIntersect(a, b []int) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func hasMsgType(s sdk.TrafficSubscription, typ string) bool {
	for _, t := range s.MessageTypes {
		if strings.EqualFold(t, typ) {
			return true
		}
	}
	return false
}

func reqHits(s sdk.TrafficSubscription, filters []nostr.Filter) bool {
	for i := range filters {
		f := &filters[i]
		if kindsIntersect(s.Kinds, f.Kinds) {
			return true
		}
		if s.ReqHasSearch && f.HasSearch() {
			return true
		}
		for _, name := range s.ReqTagNames {
			key := name
			if !strings.HasPrefix(key, "#") {
				key = "#" + key
			}
			letter := strings.TrimPrefix(key, "#")
			if vals, ok := f.Tag[key]; ok && len(vals) > 0 {
				return true
			}
			if vals, ok := f.Tag[letter]; ok && len(vals) > 0 {
				return true
			}
		}
	}
	return false
}

// MatchObserve reports whether any subscription wants a listen copy of this inbound message.
func MatchObserve(subs []sdk.TrafficSubscription, msg any) bool {
	switch m := msg.(type) {
	case *nostr.EventMessage:
		for _, s := range subs {
			if !s.Observe || !hasMsgType(s, "EVENT") {
				continue
			}
			if kindIn(s.Kinds, m.Event.Kind) {
				return true
			}
		}
	case *nostr.ReqMessage:
		for _, s := range subs {
			if !s.Observe || !hasMsgType(s, "REQ") {
				continue
			}
			if reqHits(s, m.Filters) {
				return true
			}
		}
	case *nostr.CloseMessage:
		for _, s := range subs {
			if s.Observe && hasMsgType(s, "CLOSE") {
				return true
			}
		}
	case *nostr.AuthMessage:
		for _, s := range subs {
			if s.Observe && hasMsgType(s, "AUTH") {
				return true
			}
		}
	}
	return false
}

// MatchInterceptREQ reports whether any subscription wants a sync REQ intercept.
func MatchInterceptREQ(subs []sdk.TrafficSubscription, filters []nostr.Filter) bool {
	for _, s := range subs {
		if !s.InterceptREQ || !hasMsgType(s, "REQ") {
			continue
		}
		if reqHits(s, filters) {
			return true
		}
	}
	return false
}

// MatchOnStoredEvent reports whether the plugin should be notified of an accepted event.
func MatchOnStoredEvent(subs []sdk.TrafficSubscription, kind int) bool {
	for _, s := range subs {
		if s.OnStoredEvent && kindIn(s.Kinds, kind) {
			return true
		}
	}
	return false
}
