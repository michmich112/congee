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
func RegisterNIP01(s *Server, store storage.Store) {
	s.AppendValidator(EventValidatorFunc(nip01ValidateSig))
	s.AppendPostHook("nip01_broadcast_event", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		if env.Event != nil && env.Event.Kind == nip42AuthEventKind {
			return nil
		}
		s.broadcastEvent(env.Event)
		return nil
	})
	s.AppendPostHook("nip01_audit_event", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		action := audit.ActionEventEphemeral
		if env.Stored {
			action = audit.ActionEventStored
		}
		detail := fmt.Sprintf("event_id=%s conn_id=%s kind=%d", env.Event.ID, env.Conn.ID, env.Event.Kind)
		audit.Enqueue(storage.AuditEntry{
			CreatedAt: time.Now().Unix(),
			Action:    action,
			Detail:    detail,
			Pubkey:    env.Event.PubKey,
		})
		return nil
	})
	s.RegisterMessageHandler("EVENT", func(ctx context.Context, c *Conn, msg any) error {
		return handleEVENT(ctx, s, store, c, msg.(*nostr.EventMessage))
	})
	s.RegisterMessageHandler("REQ", func(ctx context.Context, c *Conn, msg any) error {
		return handleREQ(ctx, s, c, msg.(*nostr.ReqMessage), false)
	})
	s.RegisterMessageHandler("CLOSE", func(ctx context.Context, c *Conn, msg any) error {
		handleCLOSE(ctx, s, c, msg.(*nostr.CloseMessage))
		return nil
	})
}

func nip01ValidateSig(ctx context.Context, _ *Conn, ev *nostr.Event) error {
	_ = ctx
	return ev.VerifySig()
}

func logEventRejected(ctx context.Context, c *Conn, ev *nostr.Event, reason string) {
	_ = ctx
	detail := fmt.Sprintf("event_id=%s conn_id=%s reason=%s kind=%d",
		ev.ID, c.ID, audit.SanitizeAuditDetailFragment(reason), ev.Kind)
	audit.Enqueue(storage.AuditEntry{
		CreatedAt: time.Now().Unix(),
		Action:    audit.ActionEventRejected,
		Detail:    detail,
		Pubkey:    ev.PubKey,
	})
}

func handleEVENT(ctx context.Context, s *Server, store storage.Store, c *Conn, msg *nostr.EventMessage) error {
	log := relayLogger(c, ctx)
	ev := &msg.Event
	log.Info().Str("pubkey", ev.PubKey).Int("kind", ev.Kind).Msg("event received")
	if err := s.validators.Validate(ctx, c, ev); err != nil {
		if s.metrics != nil {
			s.metrics.IncEventsRejected()
		}
		reason := err.Error()
		san := audit.SanitizeAuditDetailFragment(reason)
		log.Warn().Str("audit_action", "event_rejected").Str("event_id", ev.ID).Str("pubkey", ev.PubKey).
			Int("kind", ev.Kind).Str("reason", san).Msg("event rejected")
		logEventRejected(ctx, c, ev, reason)
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
			log.Error().Err(err).Str("pubkey", ev.PubKey).Msg("post-hook error")
		}
		return nil
	}
	if err := store.SaveEvent(ctx, ev); err != nil {
		if s.metrics != nil {
			s.metrics.IncEventsRejected()
		}
		reason := err.Error()
		log.Warn().Err(err).Str("audit_action", "event_rejected").Str("operation", "SaveEvent").Str("event_id", ev.ID).Str("pubkey", ev.PubKey).
			Int("kind", ev.Kind).Str("reason", audit.SanitizeAuditDetailFragment(reason)).Msg("event rejected: store save failed")
		logEventRejected(ctx, c, ev, reason)
		return c.sendOK(ev.ID, false, reason)
	}
	if s.metrics != nil {
		s.metrics.IncEventsStoredOK()
	}
	env := HookEnv{Conn: c, Event: ev, Stored: true}
	if err := s.hooks.Run(ctx, env); err != nil {
		log.Error().Err(err).Str("pubkey", ev.PubKey).Msg("post-hook error")
	}
	if err := c.sendOK(ev.ID, true, ""); err != nil {
		return err
	}
	return nil
}

func handleREQ(ctx context.Context, s *Server, c *Conn, msg *nostr.ReqMessage, searchEnabled bool) error {
	log := relayLogger(c, ctx)
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
	prevSubs := s.subs.SubCount(c.ID)
	if err := s.subs.Add(c.ID, msg.SubID, msg.Filters); err != nil {
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
	c.noteSubscriptionCount(s.subs.SubCount(c.ID))
	if s.subs.SubCount(c.ID) > prevSubs {
		log.Debug().
			Str("sub_id", msg.SubID).
			Int("subscriptions", s.subs.SubCount(c.ID)).
			Msg("subscription opened; idle timeout exempt")
	}
	pageSize := config.EffectiveQueryPageSize(s.cfg.ConnectionLimits.QueryPageSize)
	defaultLimit := config.EffectiveREQDefaultQueryLimit(s.cfg.ConnectionLimits.DefaultQueryLimit)
	state := newREQQueryState(msg.Filters, defaultLimit, searchEnabled)

	t0 := time.Now()
	events, hasMore, err := fetchREQPage(ctx, s.store, state, pageSize)
	durationMs := time.Since(t0).Milliseconds()
	if s.metrics != nil {
		s.metrics.RecordQueryLatency(time.Since(t0))
	}
	if err != nil {
		hasSearch := false
		for i := range msg.Filters {
			if msg.Filters[i].HasSearch() {
				hasSearch = true
				break
			}
		}
		LogStoreErr(log, zerolog.ErrorLevel, "REQ.QueryInitial", err, "req query failed", func(e *zerolog.Event) {
			e.Str("sub_id", msg.SubID).Int("filter_count", len(msg.Filters)).Bool("search_enabled", searchEnabled).
				Bool("filter_has_search", hasSearch).Int64("duration_ms", durationMs)
		})
		return c.sendClosed(msg.SubID, "internal error")
	}
	// NIP-17: REQ is not rejected upfront for filters that might return kind 1059; we query first.
	// Gift wraps are withheld per connection via EventVisibleToSubscription unless NIP-42 AUTH matches a p tag.
	for _, ev := range events {
		if !s.EventVisibleToSubscription(c.ID, ev) {
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
	if hasMore {
		openedUnix, _ := s.subs.SubOpenedUnix(c.ID, msg.SubID)
		job := &reqPageJob{
			connID:        c.ID,
			subID:         msg.SubID,
			openedUnix:    openedUnix,
			state:         state,
			searchEnabled: searchEnabled,
			pageSize:      pageSize,
		}
		if s.readQueue != nil && s.readQueue.Enqueue(job) {
			return nil
		}
		drainRemainingPages(ctx, s, c, msg.SubID, state, pageSize)
	}
	if err := c.sendEOSE(msg.SubID); err != nil {
		return err
	}
	s.subs.NoteSubEOSE(c.ID, msg.SubID)
	s.subs.FinishSnapshot(c.ID, msg.SubID)
	return nil
}

func handleCLOSE(ctx context.Context, s *Server, c *Conn, msg *nostr.CloseMessage) {
	_ = ctx
	c.sendClosed(msg.SubID, "")
	s.subs.Remove(c.ID, msg.SubID)
	c.noteSubscriptionCount(s.subs.SubCount(c.ID))
	if s.metrics != nil {
		s.metrics.IncClose()
	}
	cl := relayLogger(c, ctx)
	cl.Debug().
		Str("sub_id", msg.SubID).
		Int("subscriptions", s.subs.SubCount(c.ID)).
		Msg("subscription closed")
}
