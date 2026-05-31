package relay

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage"
)

const (
	coreCreatedAtMaxFutureSec = 900
	coreCreatedAtMaxPastSec   = 86400
)

func (s *Server) SetPluginRunner(r PluginRunner) { s.plugins = r }

func (s *Server) SupportedNIPs() []int {
	if s.plugins == nil {
		return []int{1, 11}
	}
	return s.plugins.SupportedNIPs()
}

func coreValidateEvent(s *Server, ev *nostr.Event) error {
	if ev == nil {
		return fmt.Errorf("invalid: nil event")
	}
	if err := ev.VerifySig(); err != nil {
		return err
	}
	maxBytes := s.cfg.WebSocket.MaxMessageBytes
	if maxBytes > 0 && len(ev.Content) > maxBytes {
		return fmt.Errorf("invalid: event content exceeds max size")
	}
	now := time.Now().Unix()
	if ev.CreatedAt > now+coreCreatedAtMaxFutureSec {
		return fmt.Errorf("invalid: created_at too far in the future")
	}
	if now-ev.CreatedAt > coreCreatedAtMaxPastSec {
		return fmt.Errorf("invalid: created_at too far in the past")
	}
	return nil
}

func mapPluginErr(c *Conn, s *Server, err error) (reason string, authChallenge bool) {
	if err == nil {
		return "", false
	}
	var rej plugin.Reject
	if errors.As(err, &rej) {
		return rej.Reason, false
	}
	var auth plugin.AuthRequired
	if errors.As(err, &auth) {
		reason := auth.Reason
		if reason == "" {
			reason = "auth required"
		}
		return reason, true
	}
	return err.Error(), strings.HasPrefix(err.Error(), "auth-required:")
}

func eventAuthGate(s *Server, c *Conn, ec *plugin.EventContext) error {
	if s.plugins != nil && s.plugins.EventRequiresAuth(context.Background(), ec) {
		if !ec.Conn.HasAuth() {
			return plugin.AuthRequired{Reason: "auth-required: publish requires authentication for this kind"}
		}
	}
	return validateNIP42PublishPolicy(s.cfg, c, ec.Event)
}

func runEventPipeline(ctx context.Context, s *Server, store storage.Store, c *Conn, ev *nostr.Event) (stored bool, rejectReason string, authChallenge bool, err error) {
	log := relayLogger(c, ctx)
	if isRelayInjected(ctx) {
		if nostr.IsEphemeral(ev.Kind) {
			return false, "", false, nil
		}
		if err := store.SaveEvent(ctx, ev); err != nil {
			return false, err.Error(), false, nil
		}
		return true, "", false, nil
	}

	if err := coreValidateEvent(s, ev); err != nil {
		return false, err.Error(), false, nil
	}

	ec := &plugin.EventContext{
		Conn:   newConnInfo(c),
		Event:  ev,
		Values: make(map[string]any),
	}

	if err := eventAuthGate(s, c, ec); err != nil {
		reason, auth := mapPluginErr(c, s, err)
		return false, reason, auth, nil
	}

	if s.plugins != nil {
		if err := s.plugins.ValidateEvent(ctx, ec); err != nil {
			reason, auth := mapPluginErr(c, s, err)
			return false, reason, auth, nil
		}
	}

	if err := s.validators.Validate(ctx, c, ev); err != nil {
		reason, auth := mapPluginErr(c, s, err)
		return false, reason, auth, nil
	}

	ephemeral := nostr.IsEphemeral(ev.Kind)
	if !ephemeral {
		if err := store.SaveEvent(ctx, ev); err != nil {
			return false, err.Error(), false, nil
		}
		stored = true
	}
	ec.Stored = stored

	if stored {
		corePostStoreDeletion(ctx, store, c, ev)
	}

	if s.plugins != nil {
		if err := s.plugins.OnEventStored(ctx, ec); err != nil {
			log.Error().Err(err).Str("pubkey", ev.PubKey).Msg("plugin post-store error")
		}
	}

	env := HookEnv{Conn: c, Event: ev, Stored: stored}
	if err := s.hooks.Run(ctx, env); err != nil {
		log.Error().Err(err).Str("pubkey", ev.PubKey).Msg("post-hook error")
	}

	if err := coreAuditEvent(ctx, store, c, ev, stored); err != nil {
		log.Error().Err(err).Msg("audit save failed for event outcome")
	}

	return stored, "", false, nil
}

func coreAuditEvent(ctx context.Context, store storage.Store, c *Conn, ev *nostr.Event, stored bool) error {
	action := audit.ActionEventEphemeral
	if stored {
		action = audit.ActionEventStored
	}
	detail := fmt.Sprintf("event_id=%s conn_id=%s kind=%d", ev.ID, c.ID, ev.Kind)
	return audit.Log(ctx, store, action, detail, ev.PubKey)
}

func runReqPipeline(ctx context.Context, s *Server, c *Conn, msg *nostr.ReqMessage) ([]SubFilterGate, error) {
	if subscribeAuthRequired(s.cfg, msg.Filters) && !c.nip42HasAnyAuth() {
		return nil, plugin.AuthRequired{Reason: "auth-required: subscription requires authentication"}
	}
	if s.plugins == nil {
		gates := make([]SubFilterGate, len(msg.Filters))
		for i, f := range msg.Filters {
			gates[i] = SubFilterGate{Filter: f}
		}
		return gates, nil
	}
	conn := newConnInfo(c)
	if s.plugins.ReqRequiresAuth(ctx, conn, msg.Filters) && !conn.HasAuth() {
		return nil, plugin.AuthRequired{Reason: "auth-required: subscription requires authentication"}
	}
	for i := range msg.Filters {
		if msg.Filters[i].HasSearch() && !nip50Enabled(s.cfg) {
			return nil, plugin.Reject{Reason: "search filter is not supported (enable NIP-50 in nips and restart)"}
		}
	}
	return s.plugins.PrepareReq(ctx, conn, msg.SubID, msg.Filters)
}

func nip50Enabled(cfg *config.Config) bool {
	return config.PluginEnabled(cfg, "nip-50")
}

func queryInitialFromGates(ctx context.Context, s *Server, gates []SubFilterGate, defaultLimit int) ([]*nostr.Event, error) {
	if len(gates) == 0 {
		return nil, nil
	}
	byID := make(map[string]*nostr.Event)
	for _, gate := range gates {
		f := applyDefaultQueryLimit([]nostr.Filter{gate.Filter}, defaultLimit)[0]
		gate.Filter = f
		var evs []*nostr.Event
		var err error
		if s.plugins != nil {
			evs, err = s.plugins.QueryInitialFilter(ctx, gate)
		} else {
			evs, err = s.store.QueryEvents(ctx, []nostr.Filter{f})
		}
		if err != nil {
			return nil, err
		}
		for _, ev := range evs {
			byID[ev.ID] = ev
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := byID[ids[i]], byID[ids[j]]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		return a.ID < b.ID
	})
	out := make([]*nostr.Event, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out, nil
}

func eventVisibleForSub(s *Server, connID string, gates []SubFilterGate, ev *nostr.Event) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, gate := range gates {
		if !gate.Filter.Matches(ev) {
			continue
		}
		if len(gate.Gates) == 0 {
			return true
		}
		allVisible := true
		for _, vg := range gate.Gates {
			rc := vg.ReqContext
			rc.Conn = s.liveConnInfo(connID)
			ok, err := vg.Visible.EventVisible(ctx, &rc, ev)
			if err != nil || !ok {
				allVisible = false
				break
			}
		}
		if allVisible {
			return true
		}
	}
	return false
}

func logEventRejected(ctx context.Context, store storage.Store, c *Conn, ev *nostr.Event, reason string) {
	log := relayLogger(c, ctx)
	detail := fmt.Sprintf("event_id=%s conn_id=%s reason=%s kind=%d",
		ev.ID, c.ID, audit.SanitizeAuditDetailFragment(reason), ev.Kind)
	if err := audit.Log(ctx, store, audit.ActionEventRejected, detail, ev.PubKey); err != nil {
		log.Error().Err(err).Msg("audit save failed for event_rejected")
	}
}
