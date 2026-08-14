package relay

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/michmich112/congee/internal/nostr"
)

const connAuditMaxSamples = 200

// connAuditPoint is one cumulative sample for admin connection charts (t, totals).
type connAuditPoint struct {
	TUnix int64 `json:"t"`
	Auth  int64 `json:"auth,omitempty"`
	Req   int64 `json:"req"`
	Ev    int64 `json:"ev"`
}

type connAuditRing struct {
	mu     sync.Mutex
	points []connAuditPoint
}

func (r *connAuditRing) append(p connAuditPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.points = append(r.points, p)
	if len(r.points) > connAuditMaxSamples {
		r.points = r.points[len(r.points)-connAuditMaxSamples:]
	}
}

func (r *connAuditRing) snapshotJSON() json.RawMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.points) == 0 {
		return json.RawMessage("[]")
	}
	cp := make([]connAuditPoint, len(r.points))
	copy(cp, r.points)
	b, err := json.Marshal(cp)
	if err != nil {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}

func (c *Conn) appendConnAuditSample() {
	if c == nil {
		return
	}
	c.connAudit.append(connAuditPoint{
		TUnix: time.Now().Unix(),
		Auth:  int64(c.authTotal.Load()),
		Req:   int64(c.reqTotal.Load()),
		Ev:    int64(c.clientEventTotal.Load()),
	})
}

func (c *Conn) noteInboundAfterParse(msg any) {
	switch msg.(type) {
	case *nostr.EventMessage:
		c.clientEventTotal.Add(1)
		c.noteClientEvent()
	case *nostr.ReqMessage:
		c.reqTotal.Add(1)
	case *nostr.AuthMessage:
		c.authTotal.Add(1)
	case *nostr.NegOpenMessage:
		c.negOpenTotal.Add(1)
	case *nostr.NegMsgMessage:
		c.negMsgTotal.Add(1)
	}
}

// SubConnAudit is subscription-level metrics for admin connection audit.
type SubConnAudit struct {
	SubID             string `json:"sub_id"`
	OpenedUnix        int64  `json:"opened_unix"`
	FilterCount       int    `json:"filter_count"`
	Kinds             []int  `json:"kinds,omitempty"`
	InitialSent       uint64 `json:"initial_events_sent"`
	InitialDropped    uint64 `json:"initial_events_dropped"`
	BroadcastEnqueued uint64 `json:"broadcast_events_enqueued"`
	BroadcastDropped  uint64 `json:"broadcast_events_dropped"`
	EOSESent          uint32 `json:"eose_sent"`
}
