package relay

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func testPluginHostConfig() *config.Config {
	return testRelayConfig()
}

func signedHostTestEvent(t *testing.T, priv *btcec.PrivateKey, kind int) *nostr.Event {
	t.Helper()
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: time.Now().Unix(),
		Kind:      kind,
		Content:   "host-api test",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestGuardedHostAPIGrantedSaveEvent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "host.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := testPluginHostConfig()
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	host := NewGuardedHostAPI(srv, st, "test-plugin", []plugin.Capability{plugin.CapWriteEvents}, nil, zerolog.Nop())
	priv, _ := btcec.NewPrivateKey()
	ev := signedHostTestEvent(t, priv, 1)

	if err := host.SaveEvent(ctx, ev); err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	ok, err := st.HasEventID(ctx, ev.ID)
	if err != nil || !ok {
		t.Fatalf("stored event missing: ok=%v err=%v", ok, err)
	}
}

func TestGuardedHostAPIUngrantedSaveEventAudited(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "host2.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := testPluginHostConfig()
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	host := NewGuardedHostAPI(srv, st, "test-plugin", nil, nil, zerolog.Nop())
	priv, _ := btcec.NewPrivateKey()
	ev := signedHostTestEvent(t, priv, 1)

	err = host.SaveEvent(ctx, ev)
	if err != plugin.ErrCapabilityNotGranted {
		t.Fatalf("SaveEvent err = %v, want ErrCapabilityNotGranted", err)
	}

	entries, err := st.QueryAuditLog(ctx, storage.AuditQuery{Action: audit.ActionPluginCapabilityDenied, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	if entries[0].Action != audit.ActionPluginCapabilityDenied {
		t.Fatalf("action = %q", entries[0].Action)
	}
	if entries[0].Detail == "" {
		t.Fatal("expected detail with plugin id and operation")
	}
}
