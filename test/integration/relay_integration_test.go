package integration_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func writeIntegrationConfig(dir, dsn string) string {
	p := filepath.Join(dir, "config.json")
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
    "max_subscriptions_per_connection": 20,
    "max_filters_per_req": 10,
    "connections_per_minute_per_ip": 60,
    "read_deadline_seconds": 60,
    "write_deadline_seconds": 30
  },
  "websocket": {
    "compression_enabled": false,
    "max_message_bytes": 1048576
  },
  "max_subscription_id_length": 128,
  "nip11": {
    "name": "CongeeTest",
    "description": "integration",
    "pubkey": "",
    "contact": "",
    "software": "https://example.com"
  },
  "nips": { "enabled": [1, 11] }
}`)
	Expect(os.WriteFile(p, body, 0o600)).To(Succeed())
	return p
}

func signedEvent(priv *btcec.PrivateKey, kind int, content string, tags [][]string) nostr.Event {
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := nostr.Event{
		PubKey:    pubHex,
		CreatedAt: time.Now().Unix(),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}
	_, _ = ev.ComputeID()
	Expect(ev.Sign(priv)).To(Succeed())
	return ev
}

var _ = Describe("Relay WebSocket and HTTP", func() {
	var (
		tmpDir  string
		cfg     *config.Config
		st      *sqlite.Store
		srv     *relay.Server
		ln      net.Listener
		baseWS  string
		baseHTTP string
		log     zerolog.Logger
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		dbPath := filepath.Join(tmpDir, "relay.db")
		cfgPath := writeIntegrationConfig(tmpDir, dbPath)
		var err error
		cfg, err = config.LoadJSON(cfgPath)
		Expect(err).NotTo(HaveOccurred())
		secPath := relayidentity.ResolvePath(cfgPath)
		rid, err := relayidentity.Load(secPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(relayidentity.ReconcileNIP11PubKey(cfg, rid)).To(Succeed())

		st, err = sqlite.Open(context.Background(), dbPath, nil)
		Expect(err).NotTo(HaveOccurred())

		log = zerolog.Nop()
		srv, err = relay.NewServer(cfg, st, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(nips.LoadEnabled(cfg, srv, st, log)).To(Succeed())

		ln, err = net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		go func() { _ = srv.Serve(ln) }()
		addr := ln.Addr().(*net.TCPAddr)
		baseWS = fmt.Sprintf("ws://127.0.0.1:%d/", addr.Port)
		baseHTTP = fmt.Sprintf("http://127.0.0.1:%d", addr.Port)
		time.Sleep(30 * time.Millisecond)
	})

	AfterEach(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = st.Close()
		_ = ln.Close()
	})

	It("accepts EVENT and responds OK", func() {
		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		ev := signedEvent(priv, 1, "note", nil)
		payload, err := json.Marshal([]any{"EVENT", ev})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, payload)).To(Succeed())

		_, data, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var msg []any
		Expect(json.Unmarshal(data, &msg)).To(Succeed())
		Expect(msg[0]).To(Equal("OK"))
		Expect(msg[1]).To(Equal(ev.ID))
		Expect(msg[2]).To(Equal(true))
	})

	It("handles REQ with historical EVENT then EOSE", func() {
		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		ev := signedEvent(priv, 1, "hist", nil)

		w1, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer w1.Close()
		p1, err := json.Marshal([]any{"EVENT", ev})
		Expect(err).NotTo(HaveOccurred())
		Expect(w1.WriteMessage(websocket.TextMessage, p1)).To(Succeed())
		_, _, err = w1.ReadMessage()
		Expect(err).NotTo(HaveOccurred())

		w2, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer w2.Close()
		f := map[string]any{"kinds": []int{1}, "authors": []string{ev.PubKey}}
		p2, err := json.Marshal([]any{"REQ", "sub1", f})
		Expect(err).NotTo(HaveOccurred())
		Expect(w2.WriteMessage(websocket.TextMessage, p2)).To(Succeed())

		_, data, err := w2.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var evmsg []any
		Expect(json.Unmarshal(data, &evmsg)).To(Succeed())
		Expect(evmsg[0]).To(Equal("EVENT"))
		Expect(evmsg[1]).To(Equal("sub1"))

		_, data, err = w2.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var eose []any
		Expect(json.Unmarshal(data, &eose)).To(Succeed())
		Expect(eose[0]).To(Equal("EOSE"))
		Expect(eose[1]).To(Equal("sub1"))
	})

	It("handles CLOSE", func() {
		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()
		p, err := json.Marshal([]any{"REQ", "x", map[string]any{"kinds": []int{1}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, p)).To(Succeed())
		_, _, err = c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())

		p2, err := json.Marshal([]any{"CLOSE", "x"})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, p2)).To(Succeed())
	})

	It("replaceable kind 0 keeps latest only in store", func() {
		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		ev1 := signedEvent(priv, 0, "first", nil)
		time.Sleep(10 * time.Millisecond)
		ev2 := signedEvent(priv, 0, "second", nil)

		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()
		for _, ev := range []nostr.Event{ev1, ev2} {
			p, _ := json.Marshal([]any{"EVENT", ev})
			Expect(c.WriteMessage(websocket.TextMessage, p)).To(Succeed())
			_, _, err := c.ReadMessage()
			Expect(err).NotTo(HaveOccurred())
		}

		ctx := context.Background()
		out, err := st.QueryEvents(ctx, []nostr.Filter{{Kinds: []int{0}, Authors: []string{ev2.PubKey}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(HaveLen(1))
		Expect(out[0].Content).To(Equal("second"))
	})

	It("ephemeral kind is OK but not stored; still fans out to subscribers", func() {
		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		ev := signedEvent(priv, 20000, "ephemeral", nil)

		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		sub, err := json.Marshal([]any{"REQ", "live", map[string]any{"kinds": []int{20000}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, sub)).To(Succeed())
		_, eoseData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var eose []any
		Expect(json.Unmarshal(eoseData, &eose)).To(Succeed())
		Expect(eose[0]).To(Equal("EOSE"))

		p, err := json.Marshal([]any{"EVENT", ev})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, p)).To(Succeed())
		_, okData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var okmsg []any
		Expect(json.Unmarshal(okData, &okmsg)).To(Succeed())
		Expect(okmsg[2]).To(Equal(true))

		_, evData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var wire []any
		Expect(json.Unmarshal(evData, &wire)).To(Succeed())
		Expect(wire[0]).To(Equal("EVENT"))

		ctx := context.Background()
		stored, err := st.QueryEvents(ctx, []nostr.Filter{{Kinds: []int{20000}, IDs: []string{ev.ID}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(BeEmpty())
	})

	It("serves NIP-11 on GET / with Accept application/nostr+json", func() {
		req, err := http.NewRequest(http.MethodGet, baseHTTP+"/", nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Accept", "application/nostr+json")
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Access-Control-Allow-Origin")).To(BeEmpty())
		var doc map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&doc)).To(Succeed())
		Expect(doc["name"]).To(Equal("CongeeTest"))
	})

	It("adds CORS for NIP-11 when nip11.cors_allow_any_origin is true", func() {
		tmp := GinkgoT().TempDir()
		dbPath := filepath.Join(tmp, "cors.db")
		cfgPath := filepath.Join(tmp, "config.json")
		body := []byte(`{
  "relay": { "port": 3334 },
  "admin": { "port": 3335 },
  "database": { "type": "sqlite", "dsn": "` + dbPath + `" },
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
    "max_subscriptions_per_connection": 20,
    "max_filters_per_req": 10,
    "connections_per_minute_per_ip": 60,
    "read_deadline_seconds": 60,
    "write_deadline_seconds": 30
  },
  "websocket": {
    "compression_enabled": false,
    "max_message_bytes": 1048576
  },
  "max_subscription_id_length": 128,
  "nip11": {
    "name": "CorsRelay",
    "description": "cors test",
    "pubkey": "",
    "contact": "",
    "software": "https://example.com",
    "cors_allow_any_origin": true
  },
  "nips": { "enabled": [1, 11] }
}`)
		Expect(os.WriteFile(cfgPath, body, 0o600)).To(Succeed())
		corsCfg, err := config.LoadJSON(cfgPath)
		Expect(err).NotTo(HaveOccurred())
		corsSec := relayidentity.ResolvePath(cfgPath)
		corsRid, err := relayidentity.Load(corsSec)
		Expect(err).NotTo(HaveOccurred())
		Expect(relayidentity.ReconcileNIP11PubKey(corsCfg, corsRid)).To(Succeed())
		corsSt, err := sqlite.Open(context.Background(), dbPath, nil)
		Expect(err).NotTo(HaveOccurred())
		defer corsSt.Close()
		corsSrv, err := relay.NewServer(corsCfg, corsSt, zerolog.Nop())
		Expect(err).NotTo(HaveOccurred())
		Expect(nips.LoadEnabled(corsCfg, corsSrv, corsSt, zerolog.Nop())).To(Succeed())
		corsLn, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer corsLn.Close()
		go func() { _ = corsSrv.Serve(corsLn) }()
		corsBase := fmt.Sprintf("http://127.0.0.1:%d", corsLn.Addr().(*net.TCPAddr).Port)
		time.Sleep(30 * time.Millisecond)

		getReq, err := http.NewRequest(http.MethodGet, corsBase+"/", nil)
		Expect(err).NotTo(HaveOccurred())
		getReq.Header.Set("Accept", "application/nostr+json")
		getResp, err := http.DefaultClient.Do(getReq)
		Expect(err).NotTo(HaveOccurred())
		defer getResp.Body.Close()
		Expect(getResp.StatusCode).To(Equal(http.StatusOK))
		Expect(getResp.Header.Get("Access-Control-Allow-Origin")).To(Equal("*"))
		Expect(getResp.Header.Get("Access-Control-Allow-Private-Network")).To(Equal("true"))

		optReq, err := http.NewRequest(http.MethodOptions, corsBase+"/", nil)
		Expect(err).NotTo(HaveOccurred())
		optReq.Header.Set("Origin", "https://example.org")
		optReq.Header.Set("Access-Control-Request-Method", "GET")
		optReq.Header.Set("Access-Control-Request-Headers", "accept")
		optResp, err := http.DefaultClient.Do(optReq)
		Expect(err).NotTo(HaveOccurred())
		defer optResp.Body.Close()
		Expect(optResp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(optResp.Header.Get("Access-Control-Allow-Origin")).To(Equal("*"))
		Expect(optResp.Header.Get("Access-Control-Allow-Methods")).To(ContainSubstring("GET"))
		Expect(optResp.Header.Get("Access-Control-Allow-Headers")).To(Equal("accept"))

		optReq2, err := http.NewRequest(http.MethodOptions, corsBase+"/", nil)
		Expect(err).NotTo(HaveOccurred())
		optReq2.Header.Set("Origin", "https://example.org")
		optReq2.Header.Set("Access-Control-Request-Method", "GET")
		optReq2.Header.Set("Access-Control-Request-Headers", "accept, x-custom-header")
		optResp2, err := http.DefaultClient.Do(optReq2)
		Expect(err).NotTo(HaveOccurred())
		defer optResp2.Body.Close()
		Expect(optResp2.Header.Get("Access-Control-Allow-Headers")).To(Equal("accept, x-custom-header"))

		optPNA, err := http.NewRequest(http.MethodOptions, corsBase+"/", nil)
		Expect(err).NotTo(HaveOccurred())
		optPNA.Header.Set("Origin", "https://www.nostrdeck.com")
		optPNA.Header.Set("Access-Control-Request-Method", "GET")
		optPNA.Header.Set("Access-Control-Request-Headers", "accept")
		optPNA.Header.Set("Access-Control-Request-Private-Network", "true")
		pnaResp, err := http.DefaultClient.Do(optPNA)
		Expect(err).NotTo(HaveOccurred())
		defer pnaResp.Body.Close()
		Expect(pnaResp.Header.Get("Access-Control-Allow-Private-Network")).To(Equal("true"))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Expect(corsSrv.Shutdown(ctx)).To(Succeed())
	})

	It("serves GET /health", func() {
		resp, err := http.Get(baseHTTP + "/health")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
