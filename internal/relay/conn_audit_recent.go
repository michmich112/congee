package relay

import (
	"sync"
)

// connAuditRecentClosedMax is the maximum recently closed sessions kept in relay memory.
const connAuditRecentClosedMax = 1000

// ConnAuditClosedSummary is a disconnected WebSocket session (admin connections list).
type ConnAuditClosedSummary struct {
	ConnAuditLiveSummary
	EndedUnix int64 `json:"ended_unix"`
}

// ConnAuditClosedDetail is a recent closed session with per-subscription metrics.
type ConnAuditClosedDetail struct {
	ConnAuditClosedSummary
	SubscriptionDetails []SubConnAudit `json:"subscription_details,omitempty"`
}

type connAuditRecentClosed struct {
	mu    sync.RWMutex
	items []ConnAuditClosedDetail // newest first
}

func (r *connAuditRecentClosed) add(rec ConnAuditClosedDetail) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append([]ConnAuditClosedDetail{rec}, r.items...)
	if len(r.items) > connAuditRecentClosedMax {
		r.items = r.items[:connAuditRecentClosedMax]
	}
}

func (r *connAuditRecentClosed) summaries() []ConnAuditClosedSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnAuditClosedSummary, len(r.items))
	for i := range r.items {
		out[i] = r.items[i].ConnAuditClosedSummary
	}
	return out
}

func (r *connAuditRecentClosed) detailByConnID(connID string) (*ConnAuditClosedDetail, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.items {
		if r.items[i].ConnID == connID {
			cp := r.items[i]
			return &cp, true
		}
	}
	return nil, false
}

func (s *Server) recordRecentClosedSession(c *Conn, endedUnix int64) {
	if s == nil || c == nil {
		return
	}
	c.appendConnAuditSample()
	subs := s.subs.AuditSubscriptionsForConn(c.ID)
	s.recentClosed.add(ConnAuditClosedDetail{
		ConnAuditClosedSummary: ConnAuditClosedSummary{
			ConnAuditLiveSummary: ConnAuditLiveSummary{
				Ref:               "recent:" + c.ID,
				ConnID:            c.ID,
				PeerIP:            c.peerIP,
				RemoteAddr:        c.remoteAddr,
				StartedUnix:       c.startedUnix,
				SubscriptionCount: len(subs),
				TotalAuth:         int64(c.authTotal.Load()),
				TotalReq:          int64(c.reqTotal.Load()),
				TotalClientEvent:  int64(c.clientEventTotal.Load()),
				Series:            c.connAudit.snapshotJSON(),
			},
			EndedUnix: endedUnix,
		},
		SubscriptionDetails: subs,
	})
}

// ConnAuditRecentClosedSummaries returns up to connAuditRecentClosedMax newest disconnects (newest first).
func (s *Server) ConnAuditRecentClosedSummaries() []ConnAuditClosedSummary {
	if s == nil {
		return nil
	}
	return s.recentClosed.summaries()
}

// ConnAuditRecentClosedDetailByConnID returns in-memory detail for a recent disconnect, or false if evicted.
func (s *Server) ConnAuditRecentClosedDetailByConnID(connID string) (*ConnAuditClosedDetail, bool) {
	if s == nil {
		return nil, false
	}
	return s.recentClosed.detailByConnID(connID)
}
