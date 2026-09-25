package relay

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

func TestValidateNegFilterDoesNotRevealGiftWrapIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		filter  nostr.Filter
		blocked bool
	}{
		{"wildcard", nostr.Filter{}, true},
		{"ids only", nostr.Filter{IDs: []string{strings.Repeat("a", 64)}}, true},
		{"kind 1059", nostr.Filter{Kinds: []int{nip17KindGiftWrap}}, true},
		{"kind 21059", nostr.Filter{Kinds: []int{nip59KindEphemeralGiftWrap}}, true},
		{"mixed 1059", nostr.Filter{Kinds: []int{30402, nip17KindGiftWrap}}, true},
		{"mixed 21059", nostr.Filter{Kinds: []int{0, nip59KindEphemeralGiftWrap}}, true},
		{"product only", nostr.Filter{Kinds: []int{30402}}, false},
		{"merchant profile", nostr.Filter{Kinds: []int{0, 10002}}, false},
	}
	for _, enabled := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(tt.name+" nip17="+map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
				cfg := testRelayConfig()
				if enabled {
					cfg.NIPs.Enabled = append(cfg.NIPs.Enabled, 17, 42)
					cfg.NIP42.RelayURL = "wss://relay.example/"
				}
				err := validateNegFilter(cfg, &Conn{}, &tt.filter)
				if tt.blocked && (err == nil || !strings.HasPrefix(err.Error(), "blocked:")) {
					t.Fatalf("want blocked NEG-OPEN, got %v", err)
				}
				if !tt.blocked && err != nil {
					t.Fatalf("unexpected rejection: %v", err)
				}
			})
		}
	}
}

func TestNEGOpenGiftWrapFilterReturnsProtocolError(t *testing.T) {
	t.Parallel()
	cfg := testRelayConfig()
	cfg.NIPs.Enabled = append(cfg.NIPs.Enabled, 17, 42, 77)
	cfg.NIP42.RelayURL = "wss://relay.example/"
	srv, err := NewServer(cfg, &visibilityStoreStub{}, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := registerTestConn(t, srv, "neg-gift-wrap")
	if err := handleNEGOpen(t.Context(), srv, c, &nostr.NegOpenMessage{SubID: "s", Filter: nostr.Filter{Kinds: []int{nip17KindGiftWrap}}, InitialHex: "6161"}); err != nil {
		t.Fatal(err)
	}
	var response []any
	select {
	case b := <-c.send:
		if err := json.Unmarshal(b, &response); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("expected NEG-ERR")
	}
	if len(response) < 3 || response[0] != "NEG-ERR" || response[1] != "s" {
		t.Fatalf("unexpected NEG-OPEN response: %v", response)
	}
	reason, _ := response[2].(string)
	if !strings.HasPrefix(reason, "blocked:") {
		t.Fatalf("want blocked reason, got %q", reason)
	}
}
