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
	"strconv"
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

func writeNIP42IntegrationConfig(dir, dsn, relayWSURL string) string {
	p := filepath.Join(dir, "config-nip42.json")
	body := `{
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
    "name": "CongeeNIP42",
    "description": "integration",
    "pubkey": "",
    "contact": "",
    "software": "https://example.com"
  },
  "nip42": {
    "relay_url": ` + strconv.Quote(relayWSURL) + `,
    "send_challenge_on_connect": true,
    "created_at_skew_seconds": 600,
    "require_auth_subscribe_kinds": [4],
    "require_auth_publish_kinds": [1],
    "allowlisted_pubkeys": []
  },
  "nips": { "enabled": [1, 11, 42] }
}`
	Expect(os.WriteFile(p, []byte(body), 0o600)).To(Succeed())
	return p
}

func writeNIP29IntegrationConfig(dir, dsn string) string {
	p := filepath.Join(dir, "config-nip29.json")
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
    "name": "CongeeNIP29",
    "description": "integration",
    "pubkey": "",
    "contact": "",
    "software": "https://example.com"
  },
  "nip29": {
    "late_publication_max_past_seconds": 7200,
    "strict_previous_same_h": false
  },
  "nip42": {
    "relay_url": "",
    "send_challenge_on_connect": false,
    "created_at_skew_seconds": 600,
    "require_auth_subscribe_kinds": [],
    "require_auth_publish_kinds": [],
    "allowlisted_pubkeys": []
  },
  "nips": { "enabled": [1, 11, 29] }
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

func nip42AuthEvent(priv *btcec.PrivateKey, relayURL, challenge string, createdAt int64) nostr.Event {
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := nostr.Event{
		PubKey:    pubHex,
		CreatedAt: createdAt,
		Kind:      22242,
		Tags: [][]string{
			{"relay", relayURL},
			{"challenge", challenge},
		},
		Content: "",
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
		srv, err = relay.NewServer(cfg, st, log, rid)
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
		corsSrv, err := relay.NewServer(corsCfg, corsSt, zerolog.Nop(), corsRid)
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

var _ = Describe("NIP-42 authentication", func() {
	It("sends AUTH challenge, rejects REQ until AUTH, then accepts REQ and EVENT", func() {
		tmpDir := GinkgoT().TempDir()
		dbPath := filepath.Join(tmpDir, "nip42.db")
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close()
		port := ln.Addr().(*net.TCPAddr).Port
		relayWSURL := fmt.Sprintf("ws://127.0.0.1:%d/", port)
		cfgPath := writeNIP42IntegrationConfig(tmpDir, dbPath, relayWSURL)

		cfg, err := config.LoadJSON(cfgPath)
		Expect(err).NotTo(HaveOccurred())
		secPath := relayidentity.ResolvePath(cfgPath)
		rid, err := relayidentity.Load(secPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(relayidentity.ReconcileNIP11PubKey(cfg, rid)).To(Succeed())

		st, err := sqlite.Open(context.Background(), dbPath, nil)
		Expect(err).NotTo(HaveOccurred())
		defer st.Close()

		log := zerolog.Nop()
		srv, err := relay.NewServer(cfg, st, log, rid)
		Expect(err).NotTo(HaveOccurred())
		Expect(nips.LoadEnabled(cfg, srv, st, log)).To(Succeed())

		go func() { _ = srv.Serve(ln) }()
		baseWS := fmt.Sprintf("ws://127.0.0.1:%d/", port)
		time.Sleep(30 * time.Millisecond)

		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		_, data, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var authChal []any
		Expect(json.Unmarshal(data, &authChal)).To(Succeed())
		Expect(authChal[0]).To(Equal("AUTH"))
		challenge, ok := authChal[1].(string)
		Expect(ok).To(BeTrue())
		Expect(challenge).NotTo(BeEmpty())

		reqPayload, err := json.Marshal([]any{"REQ", "sub-dm", map[string]any{"kinds": []int{4}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, reqPayload)).To(Succeed())

		_, closedData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var closed []any
		Expect(json.Unmarshal(closedData, &closed)).To(Succeed())
		Expect(closed[0]).To(Equal("CLOSED"))
		Expect(closed[1]).To(Equal("sub-dm"))
		msg, ok := closed[2].(string)
		Expect(ok).To(BeTrue())
		Expect(msg).To(HavePrefix("auth-required:"))

		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		authEv := nip42AuthEvent(priv, relayWSURL, challenge, time.Now().Unix())
		authPayload, err := json.Marshal([]any{"AUTH", authEv})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, authPayload)).To(Succeed())

		_, okData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var okmsg []any
		Expect(json.Unmarshal(okData, &okmsg)).To(Succeed())
		Expect(okmsg[0]).To(Equal("OK"))
		Expect(okmsg[1]).To(Equal(authEv.ID))
		Expect(okmsg[2]).To(Equal(true))

		Expect(c.WriteMessage(websocket.TextMessage, reqPayload)).To(Succeed())
		_, eoseData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var eose []any
		Expect(json.Unmarshal(eoseData, &eose)).To(Succeed())
		Expect(eose[0]).To(Equal("EOSE"))
		Expect(eose[1]).To(Equal("sub-dm"))

		note := signedEvent(priv, 1, "hello", nil)
		evPayload, err := json.Marshal([]any{"EVENT", note})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, evPayload)).To(Succeed())
		_, noteOkData, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var noteOk []any
		Expect(json.Unmarshal(noteOkData, &noteOk)).To(Succeed())
		Expect(noteOk[0]).To(Equal("OK"))
		Expect(noteOk[2]).To(Equal(true))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Expect(srv.Shutdown(ctx)).To(Succeed())
	})
})

var _ = Describe("NIP-29 relay groups", func() {
	It("enforces previous and late publication for h-tagged events", func() {
		tmpDir := GinkgoT().TempDir()
		dbPath := filepath.Join(tmpDir, "nip29-int.db")
		cfgPath := writeNIP29IntegrationConfig(tmpDir, dbPath)
		cfg, err := config.LoadJSON(cfgPath)
		Expect(err).NotTo(HaveOccurred())
		secPath := relayidentity.ResolvePath(cfgPath)
		rid, err := relayidentity.Load(secPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(relayidentity.ReconcileNIP11PubKey(cfg, rid)).To(Succeed())

		st, err := sqlite.Open(context.Background(), dbPath, nil)
		Expect(err).NotTo(HaveOccurred())
		defer st.Close()

		log := zerolog.Nop()
		srv, err := relay.NewServer(cfg, st, log, rid)
		Expect(err).NotTo(HaveOccurred())
		Expect(nips.LoadEnabled(cfg, srv, st, log)).To(Succeed())

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer ln.Close()
		go func() { _ = srv.Serve(ln) }()
		baseWS := fmt.Sprintf("ws://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
		time.Sleep(30 * time.Millisecond)

		c, _, err := websocket.DefaultDialer.Dial(baseWS, nil)
		Expect(err).NotTo(HaveOccurred())
		defer c.Close()

		priv, err := btcec.NewPrivateKey()
		Expect(err).NotTo(HaveOccurred())
		ev1 := signedEvent(priv, 1, "first", [][]string{{"h", "grp1"}})
		payload1, err := json.Marshal([]any{"EVENT", ev1})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, payload1)).To(Succeed())
		_, data1, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var ok1 []any
		Expect(json.Unmarshal(data1, &ok1)).To(Succeed())
		Expect(ok1[0]).To(Equal("OK"))
		Expect(ok1[2]).To(Equal(true))

		evBad := signedEvent(priv, 1, "badprev", [][]string{{"h", "grp1"}, {"previous", "aaaaaaaa"}})
		payloadBad, err := json.Marshal([]any{"EVENT", evBad})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, payloadBad)).To(Succeed())
		_, dataBad, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var okBad []any
		Expect(json.Unmarshal(dataBad, &okBad)).To(Succeed())
		Expect(okBad[0]).To(Equal("OK"))
		Expect(okBad[2]).To(Equal(false))

		prefix := ev1.ID
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		evGood := signedEvent(priv, 1, "goodprev", [][]string{{"h", "grp1"}, {"previous", prefix}})
		payloadGood, err := json.Marshal([]any{"EVENT", evGood})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, payloadGood)).To(Succeed())
		_, dataGood, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var okGood []any
		Expect(json.Unmarshal(dataGood, &okGood)).To(Succeed())
		Expect(okGood[0]).To(Equal("OK"))
		Expect(okGood[2]).To(Equal(true))

		evLate := signedEvent(priv, 1, "late", [][]string{{"h", "grp1"}})
		evLate.CreatedAt = time.Now().Unix() - 100000
		_, _ = evLate.ComputeID()
		Expect(evLate.Sign(priv)).To(Succeed())
		payloadLate, err := json.Marshal([]any{"EVENT", evLate})
		Expect(err).NotTo(HaveOccurred())
		Expect(c.WriteMessage(websocket.TextMessage, payloadLate)).To(Succeed())
		_, dataLate, err := c.ReadMessage()
		Expect(err).NotTo(HaveOccurred())
		var okLate []any
		Expect(json.Unmarshal(dataLate, &okLate)).To(Succeed())
		Expect(okLate[0]).To(Equal("OK"))
		Expect(okLate[2]).To(Equal(false))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Expect(srv.Shutdown(ctx)).To(Succeed())
	})
})
