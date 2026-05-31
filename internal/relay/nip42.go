package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage"
)

const nip42AuthEventKind = 22242

func relayNIP42Enabled(cfg *config.Config) bool {
	return cfg != nil && cfg.NIP42.Enabled
}

func nip42CreatedAtSkew(cfg *config.Config) int {
	if cfg == nil || cfg.NIP42.CreatedAtSkewSeconds <= 0 {
		return 600
	}
	return cfg.NIP42.CreatedAtSkewSeconds
}

// subscribeAuthRequired reports whether any filter may receive events of kinds that require auth.
func subscribeAuthRequired(cfg *config.Config, filters []nostr.Filter) bool {
	if !relayNIP42Enabled(cfg) {
		return false
	}
	req := cfg.NIP42.RequireAuthSubscribeKinds
	if len(req) == 0 {
		return false
	}
	for i := range filters {
		f := &filters[i]
		if len(f.Kinds) == 0 {
			return true
		}
		for _, k := range f.Kinds {
			if slices.Contains(req, k) {
				return true
			}
		}
	}
	return false
}

func validateNIP42PublishPolicy(cfg *config.Config, c *Conn, ev *nostr.Event) error {
	if !relayNIP42Enabled(cfg) || ev == nil {
		return nil
	}
	req := cfg.NIP42.RequireAuthPublishKinds
	if len(req) == 0 && len(cfg.NIP42.AllowlistedPubkeys) == 0 {
		return nil
	}
	if !slices.Contains(req, ev.Kind) {
		return nil
	}
	if !c.nip42HasPubkey(ev.PubKey) {
		return plugin.AuthRequired{Reason: "auth-required: publish requires authentication for this kind"}
	}
	if len(cfg.NIP42.AllowlistedPubkeys) > 0 {
		if !slices.Contains(cfg.NIP42.AllowlistedPubkeys, ev.PubKey) {
			return plugin.Reject{Reason: "restricted: pubkey is not allowlisted to publish"}
		}
	}
	return nil
}

func verifyNIP42AuthEvent(cfg *config.Config, ev *nostr.Event, challenge string, now time.Time) error {
	if ev.Kind != nip42AuthEventKind {
		return fmt.Errorf("invalid authentication event: want kind %d", nip42AuthEventKind)
	}
	if err := ev.VerifySig(); err != nil {
		return err
	}
	wantRelay, err := config.NormalizeNIP42RelayURL(cfg.NIP42.RelayURL)
	if err != nil {
		return fmt.Errorf("relay URL misconfigured: %w", err)
	}
	gotRelay, err := config.NormalizeNIP42RelayURL(tagFirst(ev.Tags, "relay"))
	if err != nil || gotRelay == "" {
		return fmt.Errorf("invalid or missing relay tag")
	}
	if gotRelay != wantRelay {
		return fmt.Errorf("relay tag does not match configured relay URL")
	}
	gotCh := tagFirst(ev.Tags, "challenge")
	if gotCh == "" || gotCh != challenge {
		return fmt.Errorf("challenge mismatch")
	}
	skew := int64(nip42CreatedAtSkew(cfg))
	dt := ev.CreatedAt - now.Unix()
	if dt < 0 {
		dt = -dt
	}
	if dt > skew {
		return fmt.Errorf("created_at outside allowed skew")
	}
	return nil
}

func tagFirst(tags [][]string, name string) string {
	for _, t := range tags {
		if len(t) >= 2 && t[0] == name {
			return t[1]
		}
	}
	return ""
}

// nip42EnsureChallengeLocked returns the connection challenge, generating one if
// needed. Caller must hold c.authMu (write lock).
func (c *Conn) nip42EnsureChallengeLocked() string {
	if c.nip42Challenge == "" {
		var b [24]byte
		_, _ = rand.Read(b[:])
		c.nip42Challenge = hex.EncodeToString(b[:])
	}
	return c.nip42Challenge
}

// nip42EnqueueAuthChallenge ensures this connection has a challenge and sends
// ["AUTH", challenge] at most once per connection (until the connection ends).
// Used on connect when configured, and on auth-required responses when
// send_challenge_on_connect is false.
func nip42EnqueueAuthChallenge(c *Conn, cfg *config.Config) error {
	if !relayNIP42Enabled(cfg) || c == nil {
		return nil
	}
	c.authMu.Lock()
	if c.nip42AuthSent {
		c.authMu.Unlock()
		return nil
	}
	ch := c.nip42EnsureChallengeLocked()
	c.nip42AuthSent = true
	c.authMu.Unlock()
	b, err := nostr.MarshalRelayAuth(ch)
	if err != nil {
		return err
	}
	return c.enqueue(b)
}

// RegisterNIP42 registers AUTH handling (publish policy runs in EVENT auth-gate).
func RegisterNIP42(s *Server, _ storage.Store) {
	s.RegisterMessageHandler("AUTH", func(ctx context.Context, c *Conn, msg any) error {
		return handleNIP42AUTH(ctx, s, c, msg.(*nostr.AuthMessage))
	})
}

func handleNIP42AUTH(ctx context.Context, s *Server, c *Conn, msg *nostr.AuthMessage) error {
	log := relayLogger(c, ctx)
	ev := &msg.Event
	if !relayNIP42Enabled(s.cfg) {
		return c.sendNotice("NIP-42 is not enabled")
	}
	ch := c.nip42CurrentChallenge()
	if ch == "" {
		return c.sendOK(ev.ID, false, "auth-required: no challenge was issued for this connection")
	}
	if err := verifyNIP42AuthEvent(s.cfg, ev, ch, time.Now()); err != nil {
		log.Warn().Err(err).Str("pubkey", ev.PubKey).Msg("nip42 auth rejected")
		return c.sendOK(ev.ID, false, "auth-required: "+err.Error())
	}
	c.nip42AddPubkey(ev.PubKey)
	log.Info().Str("pubkey", ev.PubKey).Msg("nip42 authenticated")
	return c.sendOK(ev.ID, true, "")
}

func (c *Conn) nip42IssueChallengeIfUnset() string {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.nip42EnsureChallengeLocked()
}

func (c *Conn) nip42CurrentChallenge() string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.nip42Challenge
}

func (c *Conn) nip42AddPubkey(pk string) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.nip42Pubkeys == nil {
		c.nip42Pubkeys = make(map[string]struct{})
	}
	c.nip42Pubkeys[pk] = struct{}{}
}

func (c *Conn) nip42HasPubkey(pk string) bool {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	_, ok := c.nip42Pubkeys[pk]
	return ok
}

func (c *Conn) nip42HasAnyAuth() bool {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return len(c.nip42Pubkeys) > 0
}

func (c *Conn) nip42AuthedPubkeys() []string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	if len(c.nip42Pubkeys) == 0 {
		return nil
	}
	out := make([]string, 0, len(c.nip42Pubkeys))
	for pk := range c.nip42Pubkeys {
		out = append(out, pk)
	}
	return out
}
