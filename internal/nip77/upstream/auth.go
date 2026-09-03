package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/rs/zerolog"
)

const nip42AuthKind = 22242

func jsonStringAt(payload []json.RawMessage, i int) string {
	if len(payload) <= i {
		return ""
	}
	var s string
	_ = json.Unmarshal(payload[i], &s)
	return s
}

func jsonBoolAt(payload []json.RawMessage, i int) bool {
	if len(payload) <= i {
		return false
	}
	var b bool
	_ = json.Unmarshal(payload[i], &b)
	return b
}

func authRelayTag(rawURL string) string {
	n, err := config.NormalizeNIP42RelayURL(rawURL)
	if err != nil {
		return rawURL
	}
	return n
}

func buildUpstreamAuthEvent(id *relayidentity.Identity, relayURL, challenge string) (*nostr.Event, error) {
	if id == nil {
		return nil, fmt.Errorf("upstream AUTH requires relay identity")
	}
	if challenge == "" {
		return nil, fmt.Errorf("empty AUTH challenge")
	}
	ev := &nostr.Event{
		CreatedAt: time.Now().Unix(),
		Kind:      nip42AuthKind,
		Tags: [][]string{
			{"relay", authRelayTag(relayURL)},
			{"challenge", challenge},
		},
		Content: "",
	}
	if err := id.SignEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (sch *Scheduler) answerAuth(c *wsClient, log zerolog.Logger, relayURL, challenge string) error {
	ev, err := buildUpstreamAuthEvent(sch.id, relayURL, challenge)
	if err != nil {
		return err
	}
	if err := c.sendJSON([]any{"AUTH", ev}); err != nil {
		return fmt.Errorf("send AUTH: %w", err)
	}
	log.Info().
		Str("relay_url", authRelayTag(relayURL)).
		Str("pubkey", sch.id.PubKeyHex()).
		Str("event_id", ev.ID).
		Msg("upstream AUTH response sent")
	return nil
}

// handshakeAuth waits briefly after connect for a NIP-42 AUTH challenge, answers it,
// and waits for OK. A timeout with no AUTH is success (relay does not require it).
func (sch *Scheduler) handshakeAuth(ctx context.Context, log zerolog.Logger, c *wsClient, relayURL string, msgTimeout time.Duration) error {
	authWait := 2 * time.Second
	if msgTimeout > 0 && msgTimeout < authWait {
		authWait = msgTimeout
	}
	deadline := time.Now().Add(authWait)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			break
		}
		typ, payload, err := c.readMessage(ctx, remaining)
		if isTimeoutErr(err) {
			log.Debug().Msg("upstream no AUTH challenge on connect")
			return nil
		}
		if err != nil {
			return err
		}
		switch typ {
		case "AUTH":
			challenge := jsonStringAt(payload, 1)
			log.Info().Str("challenge", challenge).Msg("upstream AUTH challenge")
			if err := sch.answerAuth(c, log, relayURL, challenge); err != nil {
				return err
			}
			return sch.waitAuthOK(ctx, log, c, relayURL, msgTimeout)
		case "NOTICE":
			log.Info().Str("notice", jsonStringAt(payload, 1)).Msg("upstream NOTICE during AUTH handshake")
		default:
			log.Info().Str("typ", typ).Msg("upstream message during AUTH handshake")
		}
	}
	log.Debug().Msg("upstream no AUTH challenge on connect")
	return nil
}

func (sch *Scheduler) waitAuthOK(ctx context.Context, log zerolog.Logger, c *wsClient, relayURL string, timeout time.Duration) error {
	for {
		typ, payload, err := c.readMessage(ctx, timeout)
		if err != nil {
			if isTimeoutErr(err) {
				return fmt.Errorf("timeout waiting for AUTH OK")
			}
			return err
		}
		switch typ {
		case "OK":
			evID := jsonStringAt(payload, 1)
			accepted := jsonBoolAt(payload, 2)
			msg := jsonStringAt(payload, 3)
			log.Info().Str("event_id", evID).Bool("accepted", accepted).Str("msg", msg).Msg("upstream AUTH OK")
			if !accepted {
				return fmt.Errorf("AUTH rejected: %s", msg)
			}
			return nil
		case "AUTH":
			challenge := jsonStringAt(payload, 1)
			log.Info().Str("challenge", challenge).Msg("upstream AUTH challenge")
			if err := sch.answerAuth(c, log, relayURL, challenge); err != nil {
				return err
			}
		case "NOTICE":
			log.Info().Str("notice", jsonStringAt(payload, 1)).Msg("upstream NOTICE after AUTH")
		default:
			log.Info().Str("typ", typ).Msg("upstream message while waiting for AUTH OK")
		}
	}
}
