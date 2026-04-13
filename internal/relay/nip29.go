package relay

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func nip29Enabled(cfg *config.Config) bool {
	return cfg != nil && slices.Contains(cfg.NIPs.Enabled, 29)
}

func nip29MaxPastSec(cfg *config.Config) int64 {
	if cfg == nil || cfg.NIP29.LatePublicationMaxPastSeconds <= 0 {
		return 86400
	}
	return int64(cfg.NIP29.LatePublicationMaxPastSeconds)
}

// RegisterNIP29 registers NIP-29 validators and hooks (previous, late publish, restricted write, moderation
// by relay key or latest kind-39001 admin list, open-group 9021 join flow with relay-signed 9000, 9022 with
// relay-signed 9001, private read gating).
// Remaining optional gaps: closed-group invite codes (9009 + code tag), publishing/updating 39003 roles,
// keeping 39001 in sync on every moderation action.
func RegisterNIP29(s *Server, store storage.Store, log zerolog.Logger) {
	if !nip29Enabled(s.cfg) {
		return
	}
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		_ = conn
		return nip29ValidatePrevious(ctx, store, s.cfg, ev)
	}))
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		_ = conn
		return nip29ValidateLatePublication(s.cfg, ev)
	}))
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		_ = conn
		return nip29ValidateRestrictedWrite(ctx, store, s, ev)
	}))
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		return nip29ValidateModerationAndJoinLeave(ctx, store, s, conn, ev)
	}))
	s.hooks.Prepend(func(ctx context.Context, env HookEnv) error {
		return nip29PostStoreHook(ctx, s, store, env, log)
	})
}

func nip29ValidatePrevious(ctx context.Context, store storage.Store, cfg *config.Config, ev *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(ev)
	if gid == "" {
		return nil
	}
	prevs := nostr.NIP29PreviousTagValues(ev)
	if len(prevs) == 0 {
		return nil
	}
	strict := cfg != nil && cfg.NIP29.StrictPreviousSameH
	for _, p := range prevs {
		if !nostr.NIP29IsValidPreviousPrefix(p) {
			return fmt.Errorf("nip-29: invalid previous id prefix")
		}
		ok, err := store.EventIDPrefixExists(ctx, p, gid, strict)
		if err != nil {
			return fmt.Errorf("nip-29: previous check failed: %w", err)
		}
		if !ok {
			return fmt.Errorf("nip-29: previous references unknown event")
		}
	}
	return nil
}

func nip29ValidateLatePublication(cfg *config.Config, ev *nostr.Event) error {
	if nostr.NIP29GroupHTag(ev) == "" {
		return nil
	}
	now := time.Now().Unix()
	maxPast := nip29MaxPastSec(cfg)
	if now-ev.CreatedAt > maxPast {
		return fmt.Errorf("nip-29: created_at too far in the past for this group")
	}
	return nil
}

func nip29ValidateRestrictedWrite(ctx context.Context, store storage.Store, s *Server, ev *nostr.Event) error {
	if s.relayID == nil {
		return nil
	}
	gid := nostr.NIP29GroupHTag(ev)
	if gid == "" {
		return nil
	}
	rpk := s.relayID.PubKeyHex()
	if ev.PubKey == rpk {
		return nil
	}
	if ev.Kind == nostr.NIP29KindJoinRequest || ev.Kind == nostr.NIP29KindLeaveReq {
		return nil
	}
	if nostr.NIP29IsModerationKind(ev.Kind) {
		return nil
	}
	md, err := store.GetLatestGroupMetadata39000(ctx, rpk, gid)
	if err != nil || md == nil || !nostr.NIP29MetadataIsRestricted(md) {
		return nil
	}
	member, err := store.IsGroupMember(ctx, rpk, gid, ev.PubKey)
	if err != nil {
		return fmt.Errorf("nip-29: membership check failed: %w", err)
	}
	if !member {
		return fmt.Errorf("nip-29: group is restricted: not a member")
	}
	return nil
}

func nip29ValidateModerationAndJoinLeave(ctx context.Context, store storage.Store, s *Server, _ *Conn, ev *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(ev)
	if nostr.NIP29IsModerationKind(ev.Kind) {
		if gid == "" {
			return fmt.Errorf("nip-29: moderation events require an h tag")
		}
		if s.relayID == nil {
			return fmt.Errorf("nip-29: relay identity required for moderation kinds")
		}
		rpk := s.relayID.PubKeyHex()
		if ev.PubKey == rpk {
			return nil
		}
		switch ev.Kind {
		case nostr.NIP29KindCreateGroup, nostr.NIP29KindDeleteGroup:
			return fmt.Errorf("nip-29: moderation kind %d must be signed by the relay", ev.Kind)
		default:
			admins, err := store.GetLatestGroupAdmins39001(ctx, rpk, gid)
			if err != nil {
				return fmt.Errorf("nip-29: group admins lookup failed: %w", err)
			}
			if admins == nil || !nostr.NIP29Admins39001ContainsPubkey(admins, ev.PubKey) {
				return fmt.Errorf("nip-29: moderation not permitted for this pubkey")
			}
			return nil
		}
	}
	switch ev.Kind {
	case nostr.NIP29KindJoinRequest:
		if gid == "" {
			return fmt.Errorf("nip-29: join request requires an h tag")
		}
		if s.relayID == nil {
			return fmt.Errorf("nip-29: relay identity required")
		}
		rpk := s.relayID.PubKeyHex()
		member, err := store.IsGroupMember(ctx, rpk, gid, ev.PubKey)
		if err != nil {
			return fmt.Errorf("nip-29: membership check failed: %w", err)
		}
		if member {
			return fmt.Errorf("duplicate: already a group member")
		}
		md, err := store.GetLatestGroupMetadata39000(ctx, rpk, gid)
		if err != nil {
			return fmt.Errorf("nip-29: group metadata lookup failed: %w", err)
		}
		if md != nil && nostr.NIP29MetadataIsClosed(md) {
			return fmt.Errorf("nip-29: group is closed; join requests are not accepted (invite flow not implemented)")
		}
		return nil
	case nostr.NIP29KindLeaveReq:
		if gid == "" {
			return fmt.Errorf("nip-29: leave request requires an h tag")
		}
		if s.relayID == nil {
			return fmt.Errorf("nip-29: relay identity required")
		}
		rpk := s.relayID.PubKeyHex()
		member, err := store.IsGroupMember(ctx, rpk, gid, ev.PubKey)
		if err != nil {
			return fmt.Errorf("nip-29: membership check failed: %w", err)
		}
		if !member {
			return fmt.Errorf("rejected: not a group member")
		}
		return nil
	default:
		return nil
	}
}

func nip29PostStoreHook(ctx context.Context, s *Server, store storage.Store, env HookEnv, log zerolog.Logger) error {
	if !nip29Enabled(s.cfg) || !env.Stored || env.Event == nil || s.relayID == nil {
		return nil
	}
	ev := env.Event
	gid := nostr.NIP29GroupHTag(ev)
	if gid != "" {
		if err := nip29EnsureGroupMetadata(ctx, s, store, gid); err != nil {
			log.Debug().Err(err).Str("group_id", gid).Msg("nip29 ensure group metadata failed")
		}
	}
	if ev.Kind == nostr.NIP29KindJoinRequest {
		if err := nip29RelayPutUser(ctx, s, store, ev); err != nil {
			log.Debug().Err(err).Str("group_id", gid).Msg("nip29 relay put-user after join failed")
		}
	}
	if ev.Kind == nostr.NIP29KindLeaveReq {
		if err := nip29RelayRemoveUser(ctx, s, store, ev); err != nil {
			log.Debug().Err(err).Str("group_id", gid).Msg("nip29 relay remove-user failed")
		}
	}
	return nil
}

func nip29EnsureGroupMetadata(ctx context.Context, s *Server, store storage.Store, gid string) error {
	rpk := s.relayID.PubKeyHex()
	md, err := store.GetLatestGroupMetadata39000(ctx, rpk, gid)
	if err != nil || md != nil {
		return err
	}
	now := time.Now().Unix()
	create := &nostr.Event{CreatedAt: now, Kind: nostr.NIP29KindCreateGroup, Tags: [][]string{{"h", gid}}, Content: ""}
	if err := s.relayID.SignEvent(create); err != nil {
		return err
	}
	if err := store.SaveEvent(ctx, create); err != nil {
		return err
	}
	s.broadcastEvent(create)
	meta := &nostr.Event{CreatedAt: now, Kind: nostr.NIP29KindGroupMetadata, Tags: [][]string{{"d", gid}}, Content: ""}
	if err := s.relayID.SignEvent(meta); err != nil {
		return err
	}
	if err := store.SaveEvent(ctx, meta); err != nil {
		return err
	}
	s.broadcastEvent(meta)
	return nil
}

func nip29RelayPutUser(ctx context.Context, s *Server, store storage.Store, join *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(join)
	if gid == "" {
		return nil
	}
	put := &nostr.Event{
		CreatedAt: time.Now().Unix(),
		Kind:      nostr.NIP29KindPutUser,
		Tags:      [][]string{{"h", gid}, {"p", join.PubKey}},
		Content:   "",
	}
	if err := s.relayID.SignEvent(put); err != nil {
		return err
	}
	if err := store.SaveEvent(ctx, put); err != nil {
		return err
	}
	s.broadcastEvent(put)
	return nil
}

func nip29RelayRemoveUser(ctx context.Context, s *Server, store storage.Store, leave *nostr.Event) error {
	gid := nostr.NIP29GroupHTag(leave)
	if gid == "" {
		return nil
	}
	rem := &nostr.Event{
		CreatedAt: time.Now().Unix(),
		Kind:      nostr.NIP29KindRemoveUser,
		Tags:      [][]string{{"h", gid}, {"p", leave.PubKey}},
		Content:   "",
	}
	if err := s.relayID.SignEvent(rem); err != nil {
		return err
	}
	if err := store.SaveEvent(ctx, rem); err != nil {
		return err
	}
	s.broadcastEvent(rem)
	return nil
}

func (s *Server) broadcastEvent(ev *nostr.Event) {
	if ev == nil {
		return
	}
	s.subs.Broadcast(ev, s.subscriptionEventVisible)
}

func (s *Server) subscriptionEventVisible(connID string, ev *nostr.Event) bool {
	return s.EventVisibleToSubscription(connID, ev)
}

// EventVisibleToSubscription applies NIP-29 private-group read rules using NIP-42 authenticated pubkeys on the connection.
func (s *Server) EventVisibleToSubscription(connID string, ev *nostr.Event) bool {
	if !nip29Enabled(s.cfg) || s.relayID == nil || ev == nil {
		return true
	}
	h := nostr.NIP29GroupHTag(ev)
	if h == "" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	md, err := s.store.GetLatestGroupMetadata39000(ctx, s.relayID.PubKeyHex(), h)
	if err != nil || md == nil || !nostr.NIP29MetadataIsPrivate(md) {
		return true
	}
	v, ok := s.conns.Load(connID)
	if !ok {
		return false
	}
	c := v.(*Conn)
	pks := c.nip42AuthedPubkeys()
	if len(pks) == 0 {
		return false
	}
	rpk := s.relayID.PubKeyHex()
	for _, pk := range pks {
		member, err := s.store.IsGroupMember(ctx, rpk, h, pk)
		if err == nil && member {
			return true
		}
	}
	return false
}
