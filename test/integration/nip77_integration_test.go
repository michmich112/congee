package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nip77"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func writeNIP77IntegrationConfig(dir, dsn string) string {
	p := filepath.Join(dir, "config-nip77.json")
	body := []byte(`{
  "relay": { "port": 3334 },
  "admin": { "port": 3335 },
  "database": { "type": "sqlite", "dsn": "` + dsn + `" },
  "logging": { "level": "error", "format": "json" },
  "audit": { "retention_days": 7 },
  "rate_limits": {
    "events_per_minute_per_connection": 120,
    "bytes_per_second_per_connection": 1048576,
    "reqs_per_minute_per_connection": 60,
    "messages_per_minute_per_ip": 6000
  },
  "connection_limits": {
    "max_open": 100,
    "max_open_per_ip": 20,
    "max_subscriptions_per_connection": 20,
    "max_filters_per_req": 10,
    "connections_per_minute_per_ip": 60,
    "idle_no_event_no_sub_seconds": 90,
    "read_deadline_seconds": 60,
    "write_deadline_seconds": 30
  },
  "websocket": {
    "compression_enabled": false,
    "max_message_bytes": 1048576
  },
  "max_subscription_id_length": 128,
  "nip11": {
    "name": "CongeeNIP77",
    "description": "integration",
    "pubkey": "",
    "contact": "",
    "software": "https://example.com"
  },
  "nip77": {
    "max_records_per_query": 10000,
    "session_idle_timeout_seconds": 30,
    "frame_size_limit_bytes": 1048576,
    "max_concurrent_sessions": 8,
    "max_concurrent_loads": 2,
    "neg_open_per_minute_per_connection": 60,
    "neg_msg_per_minute_per_connection": 600,
    "backpressure_req_queue_depth": 0,
    "upstream_enabled": false,
    "upstream_pause_when_busy": true,
    "upstreams": []
  },
  "nips": { "enabled": [1, 11, 77] }
}`)
	Expect(os.WriteFile(p, body, 0o600)).To(Succeed())
	return p
}

var _ = Describe("NIP-77 negentropy", func() {
	var (
		tmpDir string
		cfg    *config.Config
		st     storage.Store
		srv    *relay.Server
		ln     net.Listener
		baseWS string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		dbPath := filepath.Join(tmpDir, "relay-nip77.db")
		cfgPath := writeNIP77IntegrationConfig(tmpDir, dbPath)
		var err error
		cfg, err = config.LoadJSON(cfgPath)
		Expect(err).NotTo(HaveOccurred())
		secPath := relayidentity.ResolvePath(cfgPath)
		rid, err := relayidentity.Load(secPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(relayidentity.ReconcileNIP11PubKey(cfg, rid)).To(Succeed())

		var closeStore func() error
		st, closeStore, err = db.OpenTestStore(context.Background(), dbPath, zerolog.Nop())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = closeStore() })

		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		ev := signedEvent(priv, 1, "nip77-sync", nil)
		Expect(st.SaveEvent(context.Background(), &ev)).To(Succeed())

		srv, err = relay.NewServer(cfg, st, zerolog.Nop(), rid)
		Expect(err).NotTo(HaveOccurred())
		Expect(nips.LoadEnabled(cfg, srv, st, zerolog.Nop())).To(Succeed())

		ln, err = net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		go func() { _ = srv.Serve(ln) }()
		addr := ln.Addr().(*net.TCPAddr)
		baseWS = fmt.Sprintf("ws://127.0.0.1:%d/", addr.Port)
		time.Sleep(50 * time.Millisecond)
	})

	AfterEach(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
	})

	It("reconciles stored events over NEG-OPEN/NEG-MSG", func() {
		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		clientNeg := nip77.NewClientNegentropy(nip77.BuildVector(nil), 1<<20)
		initial := clientNeg.Start()
		filter := map[string]any{"kinds": []int{1}}
		openPayload, err := json.Marshal([]any{"NEG-OPEN", "neg1", filter, initial})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, openPayload)).To(Succeed())

		for {
			_, data, err := c.ReadMessage()
			Expect(err).NotTo(HaveOccurred())
			var raw []json.RawMessage
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			var typ string
			Expect(json.Unmarshal(raw[0], &typ)).To(Succeed())
			switch typ {
			case "NEG-ERR":
				Fail("unexpected NEG-ERR: " + string(data))
			case "NEG-MSG":
				var msgHex string
				Expect(json.Unmarshal(raw[2], &msgHex)).To(Succeed())
				out, err := clientNeg.Reconcile(strings.ToLower(msgHex))
				Expect(err).NotTo(HaveOccurred())
				if out == "" {
					closePayload, err := json.Marshal([]any{"NEG-CLOSE", "neg1"})
					Expect(err).NotTo(HaveOccurred())
					Expect(c.WriteMessage(websocket.TextMessage, closePayload)).To(Succeed())
					goto done
				}
				reply, err := json.Marshal([]any{"NEG-MSG", "neg1", out})
				Expect(err).NotTo(HaveOccurred())
				Expect(c.WriteMessage(websocket.TextMessage, reply)).To(Succeed())
			}
		}
	done:
		var need []string
		for id := range clientNeg.HaveNots {
			need = append(need, id)
		}
		Expect(need).NotTo(BeEmpty())
	})

	It("returns NEG-ERR when search filter is used", func() {
		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		clientNeg := nip77.NewClientNegentropy(nip77.BuildVector(nil), 1<<20)
		initial := clientNeg.Start()
		filter := map[string]any{"search": "test"}
		openPayload, err := json.Marshal([]any{"NEG-OPEN", "neg2", filter, initial})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, openPayload)).To(Succeed())

		_, data, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var raw []json.RawMessage
		Expect(json.Unmarshal(data, &raw)).To(Succeed())
		var typ string
		Expect(json.Unmarshal(raw[0], &typ)).To(Succeed())
		Expect(typ).To(Equal("NEG-ERR"))
		var reason string
		Expect(json.Unmarshal(raw[2], &reason)).To(Succeed())
		Expect(reason).To(ContainSubstring("blocked:"))
	})

	It("returns NEG-ERR when limit filter is used", func() {
		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		clientNeg := nip77.NewClientNegentropy(nip77.BuildVector(nil), 1<<20)
		initial := clientNeg.Start()
		filter := map[string]any{"kinds": []int{1}, "limit": 10}
		openPayload, err := json.Marshal([]any{"NEG-OPEN", "neg3", filter, initial})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, openPayload)).To(Succeed())

		_, data, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var raw []json.RawMessage
		Expect(json.Unmarshal(data, &raw)).To(Succeed())
		var typ string
		Expect(json.Unmarshal(raw[0], &typ)).To(Succeed())
		Expect(typ).To(Equal("NEG-ERR"))
		var reason string
		Expect(json.Unmarshal(raw[2], &reason)).To(Succeed())
		Expect(reason).To(ContainSubstring("blocked:"))
	})
})
