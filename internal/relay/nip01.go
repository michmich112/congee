package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// RegisterNIP01 wires NIP-01 validation, hooks, and message handlers.
func RegisterNIP01(s *Server, store storage.Store, log zerolog.Logger) {
	s.AppendValidator(EventValidatorFunc(nip01ValidateSig))
	s.AppendPostHook(func(ctx context.Context, env HookEnv) error {
		action := audit.ActionEventEphemeral
		if env.Stored {
			action = audit.ActionEventStored
		}
		detail := fmt.Sprintf("event_id=%s conn_id=%s kind=%d", env.Event.ID, env.Conn.ID, env.Event.Kind)
		return audit.Log(ctx, store, action, detail, env.Event.PubKey)
	})
	s.AppendPostHook(func(ctx context.Context, env HookEnv) error {
		_ = ctx
		if env.Event != nil && env.Event.Kind == nip42AuthEventKind {
			return nil
		}
		s.broadcastEvent(env.Event)
		return nil
	})
	s.RegisterMessageHandler("EVENT", func(ctx context.Context, c *Conn, msg any) error {
		return handleEVENT(ctx, s, store, c, msg.(*nostr.EventMessage), log)
	})
	s.RegisterMessageHandler("REQ", func(ctx context.Context, c *Conn, msg any) error {
		return handleREQ(ctx, s, c, msg.(*nostr.ReqMessage), log, false)
	})
	s.RegisterMessageHandler("CLOSE", func(ctx context.Context, c *Conn, msg any) error {
		handleCLOSE(ctx, s, c, msg.(*nostr.CloseMessage), log)
		return nil
	})
}

func nip01ValidateSig(ctx context.Context, _ *Conn, ev *nostr.Event) error {
	_ = ctx
	return ev.VerifySig()
}

func logEventRejected(ctx context.Context, store storage.Store, log zerolog.Logger, c *Conn, ev *nostr.Event, reason string) {
	detail := fmt.Sprintf("event_id=%s conn_id=%s reason=%s kind=%d",
		ev.ID, c.ID, audit.SanitizeAuditDetailFragment(reason), ev.Kind)
	if err := audit.Log(ctx, store, audit.ActionEventRejected, detail, ev.PubKey); err != nil {
		log.Error().Err(err).Str("conn_id", c.ID).Msg("audit save failed for event_rejected")
	}
}

func handleEVENT(ctx context.Context, s *Server, store storage.Store, c *Conn, msg *nostr.EventMessage, log zerolog.Logger) error {
	ev := &msg.Event
	log.Info().Str("pubkey", ev.PubKey).Int("kind", ev.Kind).Str("conn_id", c.ID).Msg("event received")
	if err := s.validators.Validate(ctx, c, ev); err != nil {
		if s.metrics != nil {
			s.metrics.IncEventsRejected()
		}
		reason := err.Error()
		logEventRejected(ctx, store, log, c, ev, reason)
		if strings.HasPrefix(reason, "auth-required:") {
			_ = nip42EnqueueAuthChallenge(c, s.cfg)
		}
		return c.sendOK(ev.ID, false, reason)
	}
	if nostr.IsEphemeral(ev.Kind) {
		if err := c.sendOK(ev.ID, true, ""); err != nil {
			return err
		}
		if s.metrics != nil {
			s.metrics.IncEventsEphemeralOK()
		}
		env := HookEnv{Conn: c, Event: ev, Stored: false}
		if err := s.hooks.Run(ctx, env); err != nil {
			log.Error().Err(err).Str("pubkey", ev.PubKey).Str("conn_id", c.ID).Msg("post-hook error")
		}
		return nil
	}
	if err := store.SaveEvent(ctx, ev); err != nil {
		if s.metrics != nil {
			s.metrics.IncEventsRejected()
		}
		reason := err.Error()
		logEventRejected(ctx, store, log, c, ev, reason)
		return c.sendOK(ev.ID, false, reason)
	}
	if s.metrics != nil {
		s.metrics.IncEventsStoredOK()
	}
	env := HookEnv{Conn: c, Event: ev, Stored: true}
	if err := s.hooks.Run(ctx, env); err != nil {
		log.Error().Err(err).Str("pubkey", ev.PubKey).Str("conn_id", c.ID).Msg("post-hook error")
	}
	if err := c.sendOK(ev.ID, true, ""); err != nil {
		return err
	}
	return nil
}

func handleREQ(ctx context.Context, s *Server, c *Conn, msg *nostr.ReqMessage, log zerolog.Logger, searchEnabled bool) error {
	if s.metrics != nil {
		s.metrics.IncReq()
	}
	for i := range msg.Filters {
		if msg.Filters[i].HasSearch() && !searchEnabled {
			return c.sendClosed(msg.SubID, "search filter is not supported (enable NIP-50 in nips.enabled and restart)")
		}
	}
	if subscribeAuthRequired(s.cfg, msg.Filters) && !c.nip42HasAnyAuth() {
		_ = nip42EnqueueAuthChallenge(c, s.cfg)
		return c.sendClosed(msg.SubID, "auth-required: subscription requires authentication")
	}
	if err := s.subs.Add(c.ID, msg.SubID, msg.Filters); err != nil {
		switch {
		case errors.Is(err, ErrSubscriptionIDTooLong):
			return c.sendClosed(msg.SubID, "subscription id too long")
		case errors.Is(err, ErrTooManyFilters):
			return c.sendClosed(msg.SubID, "too many filters")
		case errors.Is(err, ErrTooManySubscriptions):
			return c.sendClosed(msg.SubID, "too many subscriptions")
		default:
			log.Debug().Err(err).Str("conn_id", c.ID).Msg("req rejected")
			return c.sendClosed(msg.SubID, err.Error())
		}
	}
	t0 := time.Now()
	defaultLimit := config.EffectiveREQDefaultQueryLimit(s.cfg.ConnectionLimits.DefaultQueryLimit)
	events, err := queryInitialREQEvents(ctx, s.store, msg.Filters, searchEnabled, defaultLimit)
	if s.metrics != nil {
		s.metrics.RecordQueryLatency(time.Since(t0))
	}
	if err != nil {
		log.Error().Err(err).Str("conn_id", c.ID).Msg("query failed")
		return c.sendClosed(msg.SubID, "internal error")
	}
	for _, ev := range events {
		if !s.EventVisibleToSubscription(c.ID, ev) {
			continue
		}
		err := c.sendEvent(msg.SubID, ev)
		if err != nil {
			log.Debug().Err(err).Str("conn_id", c.ID).Msg("send event skipped")
		}
		s.subs.NoteSubInitialDelivery(c.ID, msg.SubID, err == nil)
	}
	if err := c.sendEOSE(msg.SubID); err != nil {
		return err
	}
	s.subs.NoteSubEOSE(c.ID, msg.SubID)
	return nil
}

func handleCLOSE(ctx context.Context, s *Server, c *Conn, msg *nostr.CloseMessage, log zerolog.Logger) {
	_ = ctx
	c.sendClosed(msg.SubID, "")
	s.subs.Remove(c.ID, msg.SubID)
	if s.metrics != nil {
		s.metrics.IncClose()
	}
	log.Debug().Str("conn_id", c.ID).Str("sub_id", msg.SubID).Msg("subscription closed")
}
