package relay

import (
	"context"
	"errors"
	"time"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// RegisterNIP01 wires NIP-01 message handlers (validation/broadcast via plugin pipeline).
func RegisterNIP01(s *Server, store storage.Store) {
	s.RegisterMessageHandler("EVENT", func(ctx context.Context, c *Conn, msg any) error {
		return handleEVENT(ctx, s, store, c, msg.(*nostr.EventMessage))
	})
	s.RegisterMessageHandler("REQ", func(ctx context.Context, c *Conn, msg any) error {
		return handleREQ(ctx, s, c, msg.(*nostr.ReqMessage))
	})
	s.RegisterMessageHandler("CLOSE", func(ctx context.Context, c *Conn, msg any) error {
		handleCLOSE(ctx, s, c, msg.(*nostr.CloseMessage))
		return nil
	})
}

func handleEVENT(ctx context.Context, s *Server, store storage.Store, c *Conn, msg *nostr.EventMessage) error {
	log := relayLogger(c, ctx)
	ev := &msg.Event
	log.Info().Str("pubkey", ev.PubKey).Int("kind", ev.Kind).Msg("event received")

	stored, reason, authChallenge, err := runEventPipeline(ctx, s, store, c, ev)
	if err != nil {
		return err
	}
	if reason != "" {
		if s.metrics != nil {
			s.metrics.IncEventsRejected()
		}
		san := audit.SanitizeAuditDetailFragment(reason)
		log.Warn().Str("audit_action", "event_rejected").Str("event_id", ev.ID).Str("pubkey", ev.PubKey).
			Int("kind", ev.Kind).Str("reason", san).Msg("event rejected")
		logEventRejected(ctx, store, c, ev, reason)
		if authChallenge {
			_ = nip42EnqueueAuthChallenge(c, s.cfg)
		}
		return c.sendOK(ev.ID, false, reason)
	}

	if stored {
		if s.metrics != nil {
			s.metrics.IncEventsStoredOK()
		}
	} else if nostr.IsEphemeral(ev.Kind) {
		if s.metrics != nil {
			s.metrics.IncEventsEphemeralOK()
		}
	}

	if err := c.sendOK(ev.ID, true, ""); err != nil {
		return err
	}
	if ev.Kind != nip42AuthEventKind {
		s.broadcastEvent(ev)
	}
	return nil
}

func handleREQ(ctx context.Context, s *Server, c *Conn, msg *nostr.ReqMessage) error {
	log := relayLogger(c, ctx)
	if s.metrics != nil {
		s.metrics.IncReq()
	}

	gates, err := runReqPipeline(ctx, s, c, msg)
	if err != nil {
		var auth plugin.AuthRequired
		if errors.As(err, &auth) {
			_ = nip42EnqueueAuthChallenge(c, s.cfg)
			reason := auth.Reason
			if reason == "" {
				reason = "auth-required: subscription requires authentication"
			}
			return c.sendClosed(msg.SubID, reason)
		}
		var rej plugin.Reject
		if errors.As(err, &rej) {
			reason := rej.Reason
			if reason == "" {
				reason = "rejected"
			}
			return c.sendClosed(msg.SubID, reason)
		}
		return c.sendClosed(msg.SubID, err.Error())
	}

	filters := make([]nostr.Filter, len(gates))
	for i := range gates {
		filters[i] = gates[i].Filter
	}

	if err := s.subs.AddWithGates(c.ID, msg.SubID, filters, gates); err != nil {
		switch {
		case errors.Is(err, ErrSubscriptionIDTooLong):
			return c.sendClosed(msg.SubID, "subscription id too long")
		case errors.Is(err, ErrTooManyFilters):
			return c.sendClosed(msg.SubID, "too many filters")
		case errors.Is(err, ErrTooManySubscriptions):
			return c.sendClosed(msg.SubID, "too many subscriptions")
		default:
			log.Warn().Err(err).Str("sub_id", msg.SubID).Msg("req subscription add rejected")
			return c.sendClosed(msg.SubID, err.Error())
		}
	}

	t0 := time.Now()
	defaultLimit := config.EffectiveREQDefaultQueryLimit(s.cfg.ConnectionLimits.DefaultQueryLimit)
	events, err := queryInitialFromGates(ctx, s, gates, defaultLimit)
	durationMs := time.Since(t0).Milliseconds()
	if s.metrics != nil {
		s.metrics.RecordQueryLatency(time.Since(t0))
	}
	if err != nil {
		LogStoreErr(log, zerolog.ErrorLevel, "REQ.QueryInitial", err, "req query failed", func(e *zerolog.Event) {
			e.Str("sub_id", msg.SubID).Int("filter_count", len(gates)).Int64("duration_ms", durationMs)
		})
		return c.sendClosed(msg.SubID, "internal error")
	}

	for _, ev := range events {
		if !eventVisibleForSub(s, c.ID, gates, ev) {
			continue
		}
		err := c.sendEvent(msg.SubID, ev)
		if err != nil {
			if errors.Is(err, ErrSlowConsumer) {
				log.Warn().Err(err).Str("sub_id", msg.SubID).Str("event_id", ev.ID).Msg("send buffer full: initial event skipped")
			} else {
				log.Debug().Err(err).Str("sub_id", msg.SubID).Str("event_id", ev.ID).Msg("send event skipped")
			}
		}
		s.subs.NoteSubInitialDelivery(c.ID, msg.SubID, err == nil)
	}
	if err := c.sendEOSE(msg.SubID); err != nil {
		return err
	}
	s.subs.NoteSubEOSE(c.ID, msg.SubID)
	return nil
}

func handleCLOSE(ctx context.Context, s *Server, c *Conn, msg *nostr.CloseMessage) {
	_ = ctx
	c.sendClosed(msg.SubID, "")
	s.subs.Remove(c.ID, msg.SubID)
	if s.metrics != nil {
		s.metrics.IncClose()
	}
	cl := relayLogger(c, ctx)
	cl.Debug().Str("sub_id", msg.SubID).Msg("subscription closed")
}
