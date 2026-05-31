package relay

import "github.com/michmich112/congee/internal/plugin"

// connInfo adapts *Conn to plugin.ConnInfo for pipeline phases.
type connInfo struct {
	c *Conn
}

func newConnInfo(c *Conn) plugin.ConnInfo {
	if c == nil {
		return nil
	}
	return connInfo{c: c}
}

// liveConnInfo returns current connection auth state for live broadcast gating.
func (s *Server) liveConnInfo(connID string) plugin.ConnInfo {
	if s == nil {
		return nil
	}
	if v, ok := s.conns.Load(connID); ok {
		if c, ok := v.(*Conn); ok {
			return newConnInfo(c)
		}
	}
	return nil
}

func (ci connInfo) ID() string { return ci.c.ID }

func (ci connInfo) PeerIP() string { return ci.c.peerIP }

func (ci connInfo) HasAuth() bool { return ci.c.nip42HasAnyAuth() }

func (ci connInfo) AuthedPubkeys() []string { return ci.c.nip42AuthedPubkeys() }
