package relay

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/nips/testfixture"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

type stubPluginRunner struct{}

func (stubPluginRunner) ValidateEvent(context.Context, *plugin.EventContext) error { return nil }
func (stubPluginRunner) OnEventStored(context.Context, *plugin.EventContext) error  { return nil }
func (stubPluginRunner) PrepareReq(context.Context, plugin.ConnInfo, string, []nostr.Filter) ([]SubFilterGate, error) {
	return nil, nil
}
func (stubPluginRunner) QueryInitialFilter(context.Context, SubFilterGate) ([]*nostr.Event, error) {
	return nil, nil
}
func (stubPluginRunner) EventVisible(context.Context, SubFilterGate, *nostr.Event) (bool, error) {
	return true, nil
}
func (stubPluginRunner) EventRequiresAuth(context.Context, *plugin.EventContext) bool { return false }
func (stubPluginRunner) ReqRequiresAuth(context.Context, plugin.ConnInfo, []nostr.Filter) bool {
	return false
}
func (stubPluginRunner) SupportedNIPs() []int { return nil }

// F2: live broadcast re-evaluates EventVisible with post-subscribe AUTH (live ConnInfo).
func TestF2LiveAuthVisibilityAfterPostSubscribeAuth(t *testing.T) {
	cfg := testRelayConfig()
	srv, err := NewServer(cfg, mustOpenStore(t), zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetPluginRunner(stubPluginRunner{})

	fx := testfixture.New()
	if err := fx.Init(NewGuardedHostAPI(srv, srv.store, "test-fixture", nil, nil, zerolog.Nop())); err != nil {
		t.Fatal(err)
	}
	vis := fx.(plugin.ReqVisibility)

	priv, _ := btcec.NewPrivateKey()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])

	connID := "conn-f2"
	c := &Conn{ID: connID, server: srv, send: make(chan []byte, 8), nip42Pubkeys: make(map[string]struct{})}
	srv.conns.Store(connID, c)

	var sent atomic.Int32
	srv.subs.RegisterSender(connID, func([]byte) bool {
		sent.Add(1)
		return true
	})

	gates := []SubFilterGate{{
		Filter: nostr.Filter{Kinds: []int{testfixture.TestKind}, Authors: []string{pubHex}},
		Gates: []VisibilityGate{{
			PluginID: "test-fixture",
			ReqContext: plugin.ReqContext{
				Conn:   newConnInfo(c),
				Values: map[string]any{"gated": true},
			},
			Visible: vis,
		}},
	}}
	if err := srv.subs.AddWithGates(connID, "sub", []nostr.Filter{gates[0].Filter}, gates); err != nil {
		t.Fatal(err)
	}

	ev := signedFixtureNote(t, priv, testfixture.TestKind, "live")
	srv.broadcastEvent(ev)
	time.Sleep(100 * time.Millisecond)
	if sent.Load() != 0 {
		t.Fatal("gated subscription must not receive events before AUTH")
	}

	c.nip42AddPubkey(pubHex)
	ev2 := signedFixtureNote(t, priv, testfixture.TestKind, "live2")
	srv.broadcastEvent(ev2)
	time.Sleep(100 * time.Millisecond)
	if sent.Load() != 1 {
		t.Fatalf("expected one delivery after AUTH, got %d", sent.Load())
	}
}

func signedFixtureNote(t *testing.T, priv *btcec.PrivateKey, kind int, content string) *nostr.Event {
	t.Helper()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	ev := &nostr.Event{PubKey: pubHex, CreatedAt: time.Now().Unix(), Kind: kind, Content: content}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}

func mustOpenStore(t *testing.T) *sqlite.Store {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "f2.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	return st
}
