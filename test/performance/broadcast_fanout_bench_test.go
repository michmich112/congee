//go:build bench

package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	sq "github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

// maxStoreCallsDuringBroadcast is the documented F1 threshold: live fan-out must not
// hit the store per subscriber (visibility uses in-memory subscription snapshots).
const maxStoreCallsDuringBroadcast = 0

type countingStore struct {
	*sq.Store
	queryCalls atomic.Int64
}

func (c *countingStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	c.queryCalls.Add(1)
	return c.Store.QueryEvents(ctx, filters)
}

var _ storage.Store = (*countingStore)(nil)

func relayBenchConfig() *config.Config {
	return &config.Config{
		ConnectionLimits: config.ConnectionLimitsSection{
			MaxOpen: 500, MaxSubscriptionsPerConnection: 500, MaxFiltersPerReq: 10,
			ConnectionsPerMinutePerIP: 10000, ReadDeadlineSeconds: 60, WriteDeadlineSeconds: 30,
		},
		WebSocket:               config.WebSocketSection{MaxMessageBytes: 1048576},
		RateLimits:              config.RateLimitsSection{EventsPerMinutePerConnection: 10000, ReqsPerMinutePerConnection: 10000, MessagesPerMinutePerIP: 100000, BytesPerSecondPerConnection: 1048576},
		MaxSubscriptionIDLength: 128,
		NIP11:                   config.NIP11Section{Name: "bench", Software: "https://example.com"},
		NIPs: map[string]config.NipPluginEntry{
			"nip-50": {Enabled: true},
		},
	}
}

// BenchmarkBroadcastFanOutF1 measures store.QueryEvents calls during live broadcast
// to many subscribers. Threshold: maxStoreCallsDuringBroadcast (0).
func BenchmarkBroadcastFanOutF1(b *testing.B) {
	ctx := context.Background()
	base, err := sq.Open(ctx, filepath.Join(b.TempDir(), "fanout.db"), nil, zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			b.Skip(err)
		}
		b.Fatal(err)
	}
	st := &countingStore{Store: base}
	defer st.Close()

	cfg := relayBenchConfig()
	srv, err := relay.NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := nips.LoadEnabled(cfg, srv, st, zerolog.Nop()); err != nil {
		b.Fatal(err)
	}
	relay.RegisterNIP01(srv, st)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
		_ = ln.Close()
	}()
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	time.Sleep(20 * time.Millisecond)

	const numSubs = 100
	conns := make([]*websocket.Conn, 0, numSubs)
	for i := 0; i < numSubs; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			b.Fatal(err)
		}
		sub, err := json.Marshal([]any{"REQ", fmt.Sprintf("s%d", i), map[string]any{"kinds": []int{1}}})
		if err != nil {
			b.Fatal(err)
		}
		if err := c.WriteMessage(websocket.TextMessage, sub); err != nil {
			b.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				b.Fatal(err)
			}
			if strings.Contains(string(data), `"EOSE"`) {
				break
			}
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	pub, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer pub.Close()

	ev := nostr.Event{
		ID: strings.Repeat("e", 64), PubKey: strings.Repeat("p", 64),
		CreatedAt: time.Now().Unix(), Kind: 1, Content: "fanout", Sig: strings.Repeat("s", 128),
	}
	payload, err := json.Marshal([]any{"EVENT", ev})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		before := st.queryCalls.Load()
		if err := pub.WriteMessage(websocket.TextMessage, payload); err != nil {
			b.Fatal(err)
		}
		if _, _, err := pub.ReadMessage(); err != nil {
			b.Fatal(err)
		}
		calls := st.queryCalls.Load() - before
		if calls > maxStoreCallsDuringBroadcast {
			b.Fatalf("F1 violation: %d store QueryEvents during broadcast (max %d)", calls, maxStoreCallsDuringBroadcast)
		}
	}
}
