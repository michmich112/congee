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
	nip17KindGiftWrap          = 1059
	nip59KindEphemeralGiftWrap = 21059
	nip17KindSeal              = 13
)

func isGiftWrapKind(kind int) bool {
	return kind == nip17KindGiftWrap || kind == nip59KindEphemeralGiftWrap
}

func nip17Enabled(cfg *config.Config) bool {
	return cfg != nil && slices.Contains(cfg.NIPs.Enabled, 17)
}

// RegisterNIP17 registers NIP-17 policy (always) and full gift-wrap handling when NIP-17 is in nips.enabled.
func RegisterNIP17(s *Server, _ storage.Store) {
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		_ = ctx
		_ = conn
		return nip17ValidateRejectGiftWrapWhenDisabled(s.cfg, ev)
	}))
	if !nip17Enabled(s.cfg) {
		return
	}
	s.AppendValidator(EventValidatorFunc(func(ctx context.Context, conn *Conn, ev *nostr.Event) error {
		_ = ctx
		_ = conn
		return nip17ValidatePublishedEvent(ev)
	}))
}

func nip17ValidateRejectGiftWrapWhenDisabled(cfg *config.Config, ev *nostr.Event) error {
	if ev == nil || !isGiftWrapKind(ev.Kind) {
		return nil
	}
	if !config.NIP17RejectGiftWrapWhenDisabled(cfg) {
		return nil
	}
	return fmt.Errorf("reject: kind %d not accepted (enable NIP-17 for private direct messages)", ev.Kind)
}

func nip17ValidatePublishedEvent(ev *nostr.Event) error {
	if ev == nil {
		return nil
	}
	switch ev.Kind {
	case nip17KindGiftWrap, nip59KindEphemeralGiftWrap:
		if _, ok := soleGiftWrapRecipient(ev); !ok {
			return fmt.Errorf("invalid gift wrap: kind %d requires exactly one canonical \"p\" recipient tag", ev.Kind)
		}
	case nip17KindSeal:
		if len(ev.Tags) != 0 {
			return fmt.Errorf("invalid seal: kind %d tags must be empty (NIP-59)", nip17KindSeal)
		}
	}
	return nil
}

func soleGiftWrapRecipient(ev *nostr.Event) (string, bool) {
	if ev == nil {
		return "", false
	}
	var recipient string
	for _, t := range ev.Tags {
		if len(t) == 0 || t[0] != "p" {
			continue
		}
		if recipient != "" || len(t) < 2 || !nip17ValidXOnlyPubKeyHex(t[1]) || strings.ToLower(t[1]) != t[1] {
			return "", false
		}
		recipient = t[1]
	}
	return recipient, recipient != ""
}

func nip17ValidXOnlyPubKeyHex(s string) bool {
	if len(s) != 64 {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32
}

// nip17GiftWrapVisibleToSubscription reports whether a NIP-59 gift wrap may be shown on this connection (NIP-17 + NIP-42).
func nip17GiftWrapVisibleToSubscription(s *Server, connID string, ev *nostr.Event) bool {
	if !relayNIP42Enabled(s.cfg) {
		return false
	}
	recipient, ok := soleGiftWrapRecipient(ev)
	if !ok {
		return false
	}
	v, ok := s.conns.Load(connID)
	if !ok {
		return false
	}
	c := v.(*Conn)
	if c.nip42HasPubkey(recipient) {
		return true
	}
	for _, pubkey := range c.nip42AuthedPubkeys() {
		if strings.EqualFold(pubkey, recipient) {
			return true
		}
	}
	return false
}
