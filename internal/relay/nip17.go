package relay

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

const (
	nip17KindGiftWrap = 1059
	nip17KindSeal     = 13
)

func nip17Enabled(cfg *config.Config) bool {
	return cfg != nil && slices.Contains(cfg.NIPs.Enabled, 17)
}

// RegisterNIP17 registers NIP-17 gift-wrap read gating (with NIP-42) and publish validators for kinds 1059 and 13.
func RegisterNIP17(s *Server, _ storage.Store) {
	if !nip17Enabled(s.cfg) {
		return
	}
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		_ = ctx
		_ = conn
		return nip17ValidatePublishedEvent(ev)
	}))
}

func nip17ValidatePublishedEvent(ev *nostr.Event) error {
	if ev == nil {
		return nil
	}
	switch ev.Kind {
	case nip17KindGiftWrap:
		if !nip17GiftWrapHasValidRecipientPTag(ev) {
			return fmt.Errorf("invalid gift wrap: kind %d requires at least one valid \"p\" recipient tag", nip17KindGiftWrap)
		}
	case nip17KindSeal:
		if len(ev.Tags) != 0 {
			return fmt.Errorf("invalid seal: kind %d tags must be empty (NIP-59)", nip17KindSeal)
		}
	}
	return nil
}

func nip17GiftWrapHasValidRecipientPTag(ev *nostr.Event) bool {
	for _, t := range ev.Tags {
		if len(t) < 2 || t[0] != "p" {
			continue
		}
		if nip17ValidXOnlyPubKeyHex(t[1]) {
			return true
		}
	}
	return false
}

func nip17ValidXOnlyPubKeyHex(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 64 {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32
}

// nip17GiftWrapRecipientPubkeys returns each valid x-only hex pubkey from "p" tags (order preserved, deduped).
func nip17GiftWrapRecipientPubkeys(ev *nostr.Event) []string {
	if ev == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, t := range ev.Tags {
		if len(t) < 2 || t[0] != "p" {
			continue
		}
		pk := strings.TrimSpace(t[1])
		if !nip17ValidXOnlyPubKeyHex(pk) {
			continue
		}
		key := strings.ToLower(pk)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pk)
	}
	return out
}

// nip17GiftWrapVisibleToSubscription reports whether a NIP-59 gift wrap may be shown on this connection (NIP-17 + NIP-42).
func nip17GiftWrapVisibleToSubscription(s *Server, connID string, ev *nostr.Event) bool {
	recipients := nip17GiftWrapRecipientPubkeys(ev)
	if len(recipients) == 0 {
		return false
	}
	v, ok := s.conns.Load(connID)
	if !ok {
		return false
	}
	c := v.(*Conn)
	authed := c.nip42AuthedPubkeys()
	if len(authed) == 0 {
		return false
	}
	for _, pk := range authed {
		for _, r := range recipients {
			if strings.EqualFold(pk, r) {
				return true
			}
		}
	}
	return false
}
