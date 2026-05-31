package relay

import (
	"context"
	"time"

	"github.com/michmich112/congee/internal/nostr"
)

func (s *Server) broadcastEvent(ev *nostr.Event) {
	if ev == nil {
		return
	}
	s.subs.Broadcast(ev, s.broadcastSubVisible)
}

func (s *Server) broadcastSubVisible(connID, subID string, entry *subEntry, ev *nostr.Event) bool {
	if s.plugins == nil || len(entry.gates) == 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, gate := range entry.gates {
		if !gate.Filter.Matches(ev) {
			continue
		}
		if len(gate.Gates) == 0 {
			return true
		}
		for _, vg := range gate.Gates {
			rc := vg.ReqContext
			rc.Conn = s.liveConnInfo(connID)
			ok, err := vg.Visible.EventVisible(ctx, &rc, ev)
			if err != nil || !ok {
				return false
			}
		}
		return true
	}
	return false
}
