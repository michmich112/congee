package relay

import (
	"context"
	"errors"
	"fmt"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// RegisterNIP01 wires NIP-01 validation, hooks, and message handlers.
func RegisterNIP01(s *Server, store storage.Store, log zerolog.Logger) {
	s.AppendValidator(EventValidatorFunc(nip01ValidateSig))
	s.AppendPostHook(func(ctx context.Context, env HookEnv) error {
		detail := fmt.Sprintf("event_id=%s conn_id=%s stored=%v kind=%d", env.Event.ID, env.Conn.ID, env.Stored, env.Event.Kind)
		return audit.Log(ctx, store, "event_accepted", detail, env.Event.PubKey)
	})
	s.AppendPostHook(func(ctx context.Context, env HookEnv) error {
		_ = ctx
		s.subs.Broadcast(env.Event)
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

func nip01ValidateSig(ctx context.Context, ev *nostr.Event) error {
	_ = ctx
	return ev.VerifySig()
}

func handleEVENT(ctx context.Context, s *Server, store storage.Store, c *Conn, msg *nostr.EventMessage, log zerolog.Logger) error {
	ev := &msg.Event
	log.Info().Str("pubkey", ev.PubKey).Int("kind", ev.Kind).Str("conn_id", c.ID).Msg("event received")
	if err := s.validators.Validate(ctx, ev); err != nil {
		return c.sendOK(ev.ID, false, err.Error())
	}
	if nostr.IsEphemeral(ev.Kind) {
		if err := c.sendOK(ev.ID, true, ""); err != nil {
			return err
		}
		env := HookEnv{Conn: c, Event: ev, Stored: false}
		if err := s.hooks.Run(ctx, env); err != nil {
			log.Error().Err(err).Str("pubkey", ev.PubKey).Str("conn_id", c.ID).Msg("post-hook error")
		}
		return nil
	}
	if err := store.SaveEvent(ctx, ev); err != nil {
		return c.sendOK(ev.ID, false, err.Error())
	}
	if err := c.sendOK(ev.ID, true, ""); err != nil {
		return err
	}
	env := HookEnv{Conn: c, Event: ev, Stored: true}
	if err := s.hooks.Run(ctx, env); err != nil {
		log.Error().Err(err).Str("pubkey", ev.PubKey).Str("conn_id", c.ID).Msg("post-hook error")
	}
	return nil
}

func handleREQ(ctx context.Context, s *Server, c *Conn, msg *nostr.ReqMessage, log zerolog.Logger, searchEnabled bool) error {
	for i := range msg.Filters {
		if msg.Filters[i].HasSearch() && !searchEnabled {
			return c.sendClosed(msg.SubID, "search filter is not supported (enable NIP-50 in nips.enabled and restart)")
		}
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
	events, err := queryInitialREQEvents(ctx, s.store, msg.Filters, searchEnabled)
	if err != nil {
		log.Error().Err(err).Str("conn_id", c.ID).Msg("query failed")
		return c.sendClosed(msg.SubID, "internal error")
	}
	for _, ev := range events {
		if err := c.sendEvent(msg.SubID, ev); err != nil {
			log.Debug().Err(err).Str("conn_id", c.ID).Msg("send event skipped")
		}
	}
	return c.sendEOSE(msg.SubID)
}

func handleCLOSE(ctx context.Context, s *Server, c *Conn, msg *nostr.CloseMessage, log zerolog.Logger) {
	_ = ctx
	s.subs.Remove(c.ID, msg.SubID)
	log.Debug().Str("conn_id", c.ID).Str("sub_id", msg.SubID).Msg("subscription closed")
}

