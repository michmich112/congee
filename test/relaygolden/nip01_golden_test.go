package relaygolden

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gorilla/websocket"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestGoldenNIP01EventAndREQ(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "golden.db")
	st, err := sqlite.Open(ctx, dbPath, nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := &config.Config{
		ConnectionLimits: config.ConnectionLimitsSection{
			MaxOpen:                       10,
			MaxSubscriptionsPerConnection: 20,
			MaxFiltersPerReq:              10,
			ConnectionsPerMinutePerIP:     60,
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
			Name: "Golden", Description: "t", Software: "https://example.com",
		},
		NIPs: make(map[string]config.NipPluginEntry),
	}

	srv, err := Start(cfg, st, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	priv, _ := btcec.PrivKeyFromBytes([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	})
	ev := signedEvent(t, priv, 1, "golden note", nil)

	// Publish event on connection 1.
	c1, _, err := websocket.DefaultDialer.Dial(srv.WSURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	evPayload, err := json.Marshal([]any{"EVENT", ev})
	if err != nil {
		t.Fatal(err)
	}
	if err := c1.WriteMessage(websocket.TextMessage, evPayload); err != nil {
		t.Fatal(err)
	}
	okFrame := readOneGolden(t, c1)

	// REQ on connection 2 (matches integration test pattern).
	c2, _, err := websocket.DefaultDialer.Dial(srv.WSURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	f := map[string]any{"kinds": []int{1}, "authors": []string{ev.PubKey}}
	reqPayload, err := json.Marshal([]any{"REQ", "sub-golden", f})
	if err != nil {
		t.Fatal(err)
	}
	if err := c2.WriteMessage(websocket.TextMessage, reqPayload); err != nil {
		t.Fatal(err)
	}
	reqFrames := readUntilGolden(t, c2, "EOSE", 8)

	frames := append([][]byte{okFrame}, reqFrames...)

	opts := NormalizeOpts{}
	got, err := NormalizeLines(frames, opts)
	if err != nil {
		t.Fatal(err)
	}
	AssertGolden(t, "nip01_event_req", got)
}

func signedEvent(t *testing.T, priv *btcec.PrivateKey, kind int, content string, tags [][]string) nostr.Event {
	t.Helper()
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := nostr.Event{
		PubKey:    pubHex,
		CreatedAt: time.Now().Unix(),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestNormalizeMessageStableID(t *testing.T) {
	stable := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	raw := []byte(`["OK","` + stable + `",true,""]`)
	opts := NormalizeOpts{StableEventIDs: map[string]struct{}{stable: {}}}
	out, err := NormalizeMessage(raw, opts)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(raw) {
		t.Fatalf("stable id rewritten: %s", out)
	}
	derived := []byte(`["OK","bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",true,""]`)
	out2, err := NormalizeMessage(derived, opts)
	if err != nil {
		t.Fatal(err)
	}
	var parsed []any
	if err := json.Unmarshal(out2, &parsed); err != nil {
		t.Fatal(err)
	}
	if id, _ := parsed[1].(string); id != placeholderEventID {
		t.Fatalf("expected placeholder id, got %q", id)
	}
}

func readOneGolden(t *testing.T, c *websocket.Conn) []byte {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readUntilGolden(t *testing.T, c *websocket.Conn, stop string, max int) [][]byte {
	t.Helper()
	var out [][]byte
	for len(out) < max {
		data := readOneGolden(t, c)
		out = append(out, data)
		var msg []any
		if json.Unmarshal(data, &msg) == nil && len(msg) > 0 && msg[0] == stop {
			return out
		}
	}
	t.Fatalf("timeout waiting for %s", stop)
	return out
}
