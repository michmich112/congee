package registry

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips/testfixture"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func testFixtureConfig(extra ...string) *config.Config {
	cfg := &config.Config{
		ConfigVersion: config.ConfigVersionCurrent,
		ConnectionLimits: config.ConnectionLimitsSection{
			MaxOpen: 50, MaxSubscriptionsPerConnection: 50, MaxFiltersPerReq: 10,
			ConnectionsPerMinutePerIP: 600, ReadDeadlineSeconds: 60, WriteDeadlineSeconds: 30,
		},
		WebSocket:               config.WebSocketSection{MaxMessageBytes: 1048576},
		RateLimits:              config.RateLimitsSection{EventsPerMinutePerConnection: 600, ReqsPerMinutePerConnection: 600, MessagesPerMinutePerIP: 60000, BytesPerSecondPerConnection: 1048576},
		MaxSubscriptionIDLength: 128,
		NIP11:                   config.NIP11Section{Name: "t", Software: "https://example.com"},
		NIPs:                    make(map[string]config.NipPluginEntry),
	}
	for _, id := range extra {
		entry := cfg.NIPs[id]
		entry.Enabled = true
		cfg.NIPs[id] = entry
	}
	return cfg
}

func loadTestRegistry(t *testing.T, cfg *config.Config, extra []plugin.Plugin) (*Registry, *sqlite.Store, *relay.Server) {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "r.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	srv, err := relay.NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prev := extraPluginFactories
	extraPluginFactories = func() []plugin.Plugin { return extra }
	t.Cleanup(func() { extraPluginFactories = prev })

	reg := &Registry{cfg: cfg, store: st, log: zerolog.Nop(), srv: srv}
	if err := reg.loadPlugins(); err != nil {
		t.Fatal(err)
	}
	srv.SetPluginRunner(reg)
	relay.RegisterNIP01(srv, st)
	return reg, st, srv
}

type stubConn struct {
	authed bool
	pks    []string
}

func (s stubConn) ID() string               { return "c1" }
func (s stubConn) PeerIP() string           { return "127.0.0.1" }
func (s stubConn) HasAuth() bool            { return s.authed }
func (s stubConn) AuthedPubkeys() []string  { return s.pks }

func TestFixtureTransformNarrowsAuthors(t *testing.T) {
	reg, _, _ := loadTestRegistry(t, testFixtureConfig(), []plugin.Plugin{testfixture.New()})
	pubA := strings.Repeat("a", 64)
	pubB := strings.Repeat("b", 64)
	gates, err := reg.PrepareReq(context.Background(), stubConn{}, "s", []nostr.Filter{{
		Kinds: []int{testfixture.TestKind}, Authors: []string{pubA, pubB},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(gates[0].Filter.Authors) != 1 || gates[0].Filter.Authors[0] != pubA {
		t.Fatalf("authors = %v, want [%s]", gates[0].Filter.Authors, pubA)
	}
	if len(gates[0].Gates) != 1 {
		t.Fatalf("visibility gates = %d", len(gates[0].Gates))
	}
}

func TestFixtureGatingSnapshotPersistsOnSubscription(t *testing.T) {
	ctx := context.Background()
	reg, st, _ := loadTestRegistry(t, testFixtureConfig(), []plugin.Plugin{testfixture.New()})

	priv, _ := btcec.NewPrivateKey()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	ev := signedFixtureEvent(t, priv, testfixture.TestKind, "stored")
	if err := st.SaveEvent(ctx, &ev); err != nil {
		t.Fatal(err)
	}

	gates, err := reg.PrepareReq(ctx, stubConn{authed: false}, "sub", []nostr.Filter{{
		Kinds: []int{testfixture.TestKind}, Authors: []string{pubHex},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(gates[0].Gates) != 1 {
		t.Fatal("expected gating snapshot on subscription filter")
	}
	vis, err := reg.EventVisible(ctx, gates[0], &ev)
	if err != nil || vis {
		t.Fatalf("without auth visible=%v err=%v", vis, err)
	}

	gatesAuthed, err := reg.PrepareReq(ctx, stubConn{authed: true, pks: []string{pubHex}}, "sub2", []nostr.Filter{{
		Kinds: []int{testfixture.TestKind}, Authors: []string{pubHex},
	}})
	if err != nil {
		t.Fatal(err)
	}
	vis2, err := reg.EventVisible(ctx, gatesAuthed[0], &ev)
	if err != nil || !vis2 {
		t.Fatalf("with auth visible=%v err=%v", vis2, err)
	}
}

func signedFixtureEvent(t *testing.T, priv *btcec.PrivateKey, kind int, content string) nostr.Event {
	t.Helper()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	ev := nostr.Event{PubKey: pubHex, CreatedAt: 1700000000, Kind: kind, Content: content}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}
