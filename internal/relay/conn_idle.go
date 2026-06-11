package relay

import (
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

const (
	// idleExemptEvent marks a connection that sent a client EVENT; never idle-timeout.
	idleExemptEvent int64 = 0
	// idleActiveSubs marks a connection with open REQ subscriptions.
	idleActiveSubs int64 = -1
)

func (c *Conn) initIdleClock() {
	if c == nil {
		return
	}
	atomic.StoreInt64(&c.idleSinceUnix, c.startedUnix)
}

// noteClientEvent exempts the connection from idle timeout after the first EVENT.
func (c *Conn) noteClientEvent() {
	if c == nil {
		return
	}
	atomic.StoreInt64(&c.idleSinceUnix, idleExemptEvent)
}

// noteSubscriptionCount updates idle tracking after subscription add/remove.
func (c *Conn) noteSubscriptionCount(n int) {
	if c == nil {
		return
	}
	if n > 0 {
		atomic.StoreInt64(&c.idleSinceUnix, idleActiveSubs)
		return
	}
	if c.clientEventTotal.Load() > 0 {
		atomic.StoreInt64(&c.idleSinceUnix, idleExemptEvent)
		return
	}
	atomic.StoreInt64(&c.idleSinceUnix, time.Now().Unix())
}

func (c *Conn) idleEligible() bool {
	if c == nil {
		return false
	}
	since := atomic.LoadInt64(&c.idleSinceUnix)
	if since <= idleExemptEvent {
		return false
	}
	if c.clientEventTotal.Load() > 0 {
		return false
	}
	return c.server.subs.SubCount(c.ID) == 0
}

func (c *Conn) idleSeconds(nowUnix int64) int64 {
	since := atomic.LoadInt64(&c.idleSinceUnix)
	if since <= idleExemptEvent {
		return 0
	}
	return nowUnix - since
}

func (s *Server) idleConnSweeper() {
	limit := s.cfg.ConnectionLimits.IdleNoEventNoSubSeconds
	if limit <= 0 {
		return
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.metricsCtx.Done():
			return
		case <-tick.C:
			s.sweepIdleConnections(limit)
		}
	}
}

func (s *Server) sweepIdleConnections(limitSec int) {
	now := time.Now().Unix()
	s.conns.Range(func(_, v any) bool {
		c := v.(*Conn)
		if !c.idleEligible() {
			return true
		}
		idleSec := c.idleSeconds(now)
		if idleSec < int64(limitSec) {
			if c.log.GetLevel() <= zerolog.DebugLevel {
				c.log.Debug().
					Str("peer_ip", c.peerIP).
					Str("remote_addr", c.remoteAddr).
					Int64("idle_seconds", idleSec).
					Int("idle_limit_seconds", limitSec).
					Int("subscriptions", c.server.subs.SubCount(c.ID)).
					Uint64("client_events", c.clientEventTotal.Load()).
					Msg("ws client idle check: below limit")
			}
			return true
		}
		if s.metrics != nil {
			s.metrics.IncIdleDisconnect()
		}
		c.log.Warn().
			Str("peer_ip", c.peerIP).
			Str("remote_addr", c.remoteAddr).
			Int64("started_unix", c.startedUnix).
			Int64("idle_since_unix", atomic.LoadInt64(&c.idleSinceUnix)).
			Int64("idle_seconds", idleSec).
			Int("idle_limit_seconds", limitSec).
			Int("subscriptions", c.server.subs.SubCount(c.ID)).
			Uint64("client_events", c.clientEventTotal.Load()).
			Uint64("req_messages", c.reqTotal.Load()).
			Uint64("auth_messages", c.authTotal.Load()).
			Msg("ws client idle timeout: no events and no subscriptions")
		c.cancel()
		_ = c.nc.Close()
		return true
	})
}
