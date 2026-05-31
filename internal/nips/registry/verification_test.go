package registry

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	nip50plugin "github.com/michmich112/congee/internal/nips/nip50"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

// F3: intersect-only transform rejects widening.
type widenTransformPlugin struct{}

func (w *widenTransformPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "widen-test", DefaultEnabled: true, Priority: 1,
		Capabilities: []plugin.Capability{plugin.CapTransformReq},
		Routes:       []plugin.Route{{Kinds: []int{880001}, Req: &plugin.DirectionPolicy{}}},
	}
}
func (w *widenTransformPlugin) Init(plugin.HostAPI) error { return nil }
func (w *widenTransformPlugin) TransformReq(_ context.Context, rc *plugin.ReqContext) error {
	if len(rc.Filters) > 0 {
		rc.Filters[0].Authors = append(rc.Filters[0].Authors, strings.Repeat("f", 64))
	}
	return nil
}

func TestF3IntersectOnlyTransformRejectsWidening(t *testing.T) {
	reg, _, _ := loadTestRegistry(t, testFixtureConfig(), []plugin.Plugin{&widenTransformPlugin{}})
	orig := strings.Repeat("a", 64)
	gates, err := reg.PrepareReq(context.Background(), stubConn{}, "s", []nostr.Filter{{
		Kinds: []int{880001}, Authors: []string{orig},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(gates[0].Filter.Authors) != 1 || gates[0].Filter.Authors[0] != orig {
		t.Fatalf("widening must be rejected; authors=%v", gates[0].Filter.Authors)
	}
}

// F4: event immutability after ValidateEvent (validators must not mutate the signed event).
type readOnlyValidatorPlugin struct{}

func (m *readOnlyValidatorPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "readonly-test", DefaultEnabled: true, Priority: 1,
		Capabilities: []plugin.Capability{plugin.CapValidateEvent},
		Routes:       []plugin.Route{{Kinds: []int{880002}, Event: &plugin.DirectionPolicy{}}},
	}
}
func (m *readOnlyValidatorPlugin) Init(plugin.HostAPI) error { return nil }
func (m *readOnlyValidatorPlugin) ValidateEvent(_ context.Context, ec *plugin.EventContext) error {
	_ = ec.Event.Content
	return nil
}

func TestF4EventImmutabilityAfterValidateEvent(t *testing.T) {
	ctx := context.Background()
	reg, _, _ := loadTestRegistry(t, testFixtureConfig(), []plugin.Plugin{&readOnlyValidatorPlugin{}})

	origContent := "original"
	priv := testPrivKey(t)
	ev := signedWithKind(t, priv, 880002, origContent)
	beforeID := ev.ID

	ec := &plugin.EventContext{Event: ev, Values: make(map[string]any)}
	if err := reg.ValidateEvent(ctx, ec); err != nil {
		t.Fatal(err)
	}
	if ev.Content != origContent || ev.ID != beforeID {
		t.Fatalf("event mutated during ValidateEvent: content=%q id=%s", ev.Content, ev.ID)
	}
	if err := ev.VerifySig(); err != nil {
		t.Fatalf("signature invalid after validate: %v", err)
	}
}

// F5: ReqQueryProvider bypasses default store query.
type queryCountStore struct {
	*sqlite.Store
	queryCalls atomic.Int64
}

func (c *queryCountStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	c.queryCalls.Add(1)
	return c.Store.QueryEvents(ctx, filters)
}

func TestF5ReqQueryProviderBypassesDefaultQuery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	base, err := sqlite.Open(ctx, filepath.Join(dir, "q.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &queryCountStore{Store: base}
	cfg := testFixtureConfig("nip-50")
	srv, err := relay.NewServer(cfg, wrapped, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prev := extraPluginFactories
	extraPluginFactories = func() []plugin.Plugin { return []plugin.Plugin{nip50plugin.New()} }
	defer func() { extraPluginFactories = prev }()
	reg := &Registry{cfg: cfg, store: wrapped, log: zerolog.Nop(), srv: srv}
	if err := reg.loadPlugins(); err != nil {
		t.Fatal(err)
	}

	q := "hello"
	gate := relay.SubFilterGate{Filter: nostr.Filter{Search: &q}}
	before := wrapped.queryCalls.Load()
	_, err = reg.QueryInitialFilter(ctx, gate)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.queryCalls.Load() != before {
		t.Fatalf("QueryEvents called during provider query, want bypass")
	}
}

// F6: multi-kind REQ auth when any matched kind requires auth.
type authKindPlugin struct{}

func (a *authKindPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "auth-kind-test", DefaultEnabled: true, Priority: 1,
		Routes: []plugin.Route{{
			Kinds: []int{880003},
			Req:   &plugin.DirectionPolicy{RequiresAuth: true},
		}},
	}
}
func (a *authKindPlugin) Init(plugin.HostAPI) error { return nil }

func TestF6MultiKindREQAuthWhenAnyKindRequiresAuth(t *testing.T) {
	reg, _, _ := loadTestRegistry(t, testFixtureConfig(), []plugin.Plugin{&authKindPlugin{}})
	f := nostr.Filter{Kinds: []int{1, 880003}}
	if !reg.ReqRequiresAuth(context.Background(), stubConn{authed: false}, []nostr.Filter{f}) {
		t.Fatal("multi-kind filter with auth-required kind must require auth")
	}
	if reg.ReqRequiresAuth(context.Background(), stubConn{authed: true, pks: []string{"x"}}, []nostr.Filter{f}) {
		t.Fatal("authed conn should not require auth")
	}
}

// F7: registry startup fails on capability/interface mismatch.
type mismatchPlugin struct{}

func (m *mismatchPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "mismatch", DefaultEnabled: true,
		Capabilities: []plugin.Capability{plugin.CapTransformReq},
		Routes:       []plugin.Route{{Kinds: []int{1}, Req: &plugin.DirectionPolicy{}}},
	}
}
func (m *mismatchPlugin) Init(plugin.HostAPI) error { return nil }

func TestF7RegistryStartupFailsOnCapabilityMismatch(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "m.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	srv, err := relay.NewServer(testFixtureConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	prev := extraPluginFactories
	extraPluginFactories = func() []plugin.Plugin { return []plugin.Plugin{&mismatchPlugin{}} }
	defer func() { extraPluginFactories = prev }()

	reg := &Registry{cfg: testFixtureConfig(), store: st, log: zerolog.Nop(), srv: srv}
	err = reg.loadPlugins()
	if err == nil {
		t.Fatal("expected startup failure for declared-but-unimplemented capability")
	}
	if !strings.Contains(err.Error(), "declares transform_req but interface not implemented") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ensure queryCountStore satisfies storage.Store for the fields we use.
var _ storage.Store = (*queryCountStore)(nil)

func testPrivKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

func signedWithKind(t *testing.T, priv *btcec.PrivateKey, kind int, content string) *nostr.Event {
	t.Helper()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey: pubHex, CreatedAt: 1700000000, Kind: kind, Content: content,
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}
