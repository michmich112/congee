package relay

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/michmich112/congee/internal/storage"
)

// ConnAuditLiveSummary is one open WebSocket connection for admin audit APIs.
type ConnAuditLiveSummary struct {
	Ref                 string          `json:"ref"`
	ConnID              string          `json:"conn_id"`
	PeerIP              string          `json:"peer_ip"`
	RemoteAddr          string          `json:"remote_addr"`
	StartedUnix         int64           `json:"started_unix"`
	SubscriptionCount   int             `json:"subscriptions"`
	TotalAuth           int64           `json:"total_auth"`
	TotalReq            int64           `json:"total_req"`
	TotalClientEvent    int64           `json:"total_client_event"`
	Series              json.RawMessage `json:"series"`
}

// ConnAuditLiveDetail is a live connection plus per-subscription metrics.
type ConnAuditLiveDetail struct {
	ConnAuditLiveSummary
	SubscriptionDetails []SubConnAudit `json:"subscription_details"`
}

// ConnAuditLiveSummaries returns all open connections (newest started last; sorted by conn_id for stability).
func (s *Server) ConnAuditLiveSummaries() []ConnAuditLiveSummary {
	var out []ConnAuditLiveSummary
	s.conns.Range(func(_, v any) bool {
		c := v.(*Conn)
		out = append(out, ConnAuditLiveSummary{
			Ref:               "live:" + c.ID,
			ConnID:            c.ID,
			PeerIP:            c.peerIP,
			RemoteAddr:        c.remoteAddr,
			StartedUnix:       c.startedUnix,
			SubscriptionCount: s.subs.SubCount(c.ID),
			TotalAuth:         int64(c.authTotal.Load()),
			TotalReq:          int64(c.reqTotal.Load()),
			TotalClientEvent:  int64(c.clientEventTotal.Load()),
			Series:            c.connAudit.snapshotJSON(),
		})
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ConnID < out[j].ConnID })
	return out
}

// ConnAuditLiveDetailByConnID returns live detail for connID, or false if not connected.
func (s *Server) ConnAuditLiveDetailByConnID(connID string) (*ConnAuditLiveDetail, bool) {
	v, ok := s.conns.Load(connID)
	if !ok {
		return nil, false
	}
	c := v.(*Conn)
	sum := ConnAuditLiveSummary{
		Ref:               "live:" + c.ID,
		ConnID:            c.ID,
		PeerIP:            c.peerIP,
		RemoteAddr:        c.remoteAddr,
		StartedUnix:       c.startedUnix,
		SubscriptionCount: s.subs.SubCount(c.ID),
		TotalAuth:         int64(c.authTotal.Load()),
		TotalReq:          int64(c.reqTotal.Load()),
		TotalClientEvent:  int64(c.clientEventTotal.Load()),
		Series:            c.connAudit.snapshotJSON(),
	}
	return &ConnAuditLiveDetail{
		ConnAuditLiveSummary: sum,
		SubscriptionDetails:  s.subs.AuditSubscriptionsForConn(connID),
	}, true
}

func (s *Server) persistConnAuditSession(c *Conn) {
	if s.store == nil || c == nil {
		return
	}
	c.appendConnAuditSample()
	subs := s.subs.AuditSubscriptionsForConn(c.ID)
	subsJSON, err := json.Marshal(subs)
	if err != nil {
		subsJSON = []byte("[]")
	}
	ent := storage.WSConnectionSession{
		ConnID:           c.ID,
		PeerIP:           c.peerIP,
		RemoteAddr:       c.remoteAddr,
		StartedUnix:      c.startedUnix,
		EndedUnix:        time.Now().Unix(),
		TotalAuth:        int64(c.authTotal.Load()),
		TotalReq:         int64(c.reqTotal.Load()),
		TotalClientEvent: int64(c.clientEventTotal.Load()),
		SeriesJSON:       c.connAudit.snapshotJSON(),
		SubsJSON:         subsJSON,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := s.store.SaveWSConnectionSession(ctx, ent); err != nil {
		c.log.Debug().Err(err).Msg("ws connection session audit persist failed")
	}
}

func (s *Server) connAuditSampler() {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.metricsCtx.Done():
			return
		case <-tick.C:
			s.conns.Range(func(_, v any) bool {
				v.(*Conn).appendConnAuditSample()
				return true
			})
		}
	}
}
