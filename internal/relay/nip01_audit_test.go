package relay

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/michmich112/congee/internal/storage/sqlitemeta"
	"github.com/rs/zerolog"
)

func testRelayConfig() *config.Config {
	return &config.Config{
		ConnectionLimits: config.ConnectionLimitsSection{
			MaxOpen:                       10,
			MaxOpenPerIP:                  20,
			MaxSubscriptionsPerConnection: 20,
			MaxFiltersPerReq:              10,
			ConnectionsPerMinutePerIP:     60,
			IdleNoEventNoSubSeconds:       90,
			ReadDeadlineSeconds:           60,
			WriteDeadlineSeconds:          30,
		},
		WebSocket: config.WebSocketSection{
			CompressionEnabled: false,
			MaxMessageBytes:    1048576,
		},
		RateLimits: config.RateLimitsSection{
			EventsPerMinutePerConnection: 120,
			BytesPerSecondPerConnection:  1048576,
			ReqsPerMinutePerConnection:   60,
			MessagesPerMinutePerIP:       6000,
		},
		MaxSubscriptionIDLength: 128,
		NIP11: config.NIP11Section{
			Name: "t", Description: "t", PubKey: "", Contact: "", Software: "https://example.com",
		},
		NIPs: config.NIPsSection{Enabled: []int{1, 11}},
	}
}

type faultEventStore struct {
	storage.EventStore
	saveErr error
}

func (f *faultEventStore) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.EventStore.SaveEvent(ctx, ev)
}

func openAuditTestStore(ctx context.Context, t *testing.T, dir, name string) (storage.Store, func() error) {
	t.Helper()
	st, closeFn, err := db.OpenTestStore(ctx, filepath.Join(dir, name), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	return st, closeFn
}

func openFaultAuditTestStore(ctx context.Context, t *testing.T, dir string, saveErr error) (storage.Store, func()) {
	t.Helper()
	meta, err := sqlitemeta.Open(ctx, filepath.Join(dir, "meta.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	ev, err := sqlite.Open(ctx, filepath.Join(dir, "r.db"), nil, zerolog.Nop())
	if err != nil {
		_ = meta.Close()
		t.Fatal(err)
	}
	st := db.NewCompositeForTest(&faultEventStore{EventStore: ev, saveErr: saveErr}, meta)
	return st, func() {
		_ = meta.Close()
		_ = ev.Close()
	}
}

func signedTestEvent(t *testing.T, priv *btcec.PrivateKey, kind int) *nostr.Event {
	t.Helper()
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: time.Now().Unix(),
		Kind:      kind,
		Content:   "x",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}

func latestAuditAction(ctx context.Context, t *testing.T, st storage.Store, action string) storage.AuditEntry {
	t.Helper()
	rows, err := st.QueryAuditLog(ctx, storage.AuditQuery{Action: action, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("no audit rows for action %q", action)
	}
	return rows[0]
}

func testConn(t *testing.T, srv *Server) *Conn {
	t.Helper()
	c := registerTestConn(t, srv, "c1")
	c.send = make(chan []byte, 64)
	return c
}

func drainOK(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case <-c.send:
	default:
		t.Fatal("expected outbound OK frame")
	}
}

func TestHandleEVENT_AuditRejectInvalidSig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore := openAuditTestStore(ctx, t, dir, "r.db")
	defer closeStore()
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	c := testConn(t, srv)

	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1,
		Kind:      1,
		Content:   "c",
		Sig:       strings.Repeat("d", 128),
	}
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(ctx, srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	drainOK(t, c)
	row := latestAuditAction(ctx, t, st, audit.ActionEventRejected)
	if row.Pubkey != ev.PubKey {
		t.Fatalf("pubkey: %s", row.Pubkey)
	}
	if !strings.Contains(row.Detail, "reason=") {
		t.Fatalf("detail missing reason: %q", row.Detail)
	}
	if !strings.HasSuffix(row.Detail, " kind=1") {
		t.Fatalf("detail suffix: %q", row.Detail)
	}
}

func TestHandleEVENT_AuditRejectSaveError(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStores := openFaultAuditTestStore(ctx, t, dir, errors.New("disk full"))
	defer closeStores()
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	c := testConn(t, srv)

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	ev := signedTestEvent(t, priv, 1)
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(ctx, srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	drainOK(t, c)
	row := latestAuditAction(ctx, t, st, audit.ActionEventRejected)
	if !strings.Contains(row.Detail, "reason=disk full") {
		t.Fatalf("detail: %q", row.Detail)
	}
}

func TestHandleEVENT_AuditStored(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore := openAuditTestStore(ctx, t, dir, "r.db")
	defer closeStore()
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	c := testConn(t, srv)

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	ev := signedTestEvent(t, priv, 1)
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(ctx, srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	drainOK(t, c)
	row := latestAuditAction(ctx, t, st, audit.ActionEventStored)
	if row.Pubkey != ev.PubKey {
		t.Fatalf("pubkey")
	}
	want := "event_id=" + ev.ID + " conn_id=c1 kind=1"
	if row.Detail != want {
		t.Fatalf("detail: got %q want %q", row.Detail, want)
	}
}

func TestHandleEVENT_AuditEphemeral(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore := openAuditTestStore(ctx, t, dir, "r.db")
	defer closeStore()
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	c := testConn(t, srv)

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	ev := signedTestEvent(t, priv, 20000)
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(ctx, srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	drainOK(t, c)
	row := latestAuditAction(ctx, t, st, audit.ActionEventEphemeral)
	if row.Pubkey != ev.PubKey {
		t.Fatalf("pubkey")
	}
	want := "event_id=" + ev.ID + " conn_id=c1 kind=20000"
	if row.Detail != want {
		t.Fatalf("detail: got %q want %q", row.Detail, want)
	}
}
