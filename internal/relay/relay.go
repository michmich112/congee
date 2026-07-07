package relay

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// Server is the Nostr relay HTTP and WebSocket front end.
type Server struct {
	cfg     *config.Config
	store   storage.Store
	log     zerolog.Logger
	relayID *relayidentity.Identity

	registry   *Registry
	validators *ValidatorChain
	hooks      *HookChain
	subs       *SubscriptionManager
	limiter    *Hub

	http *http.Server

	open   atomic.Int64
	conns  sync.Map // conn id -> *Conn
	ipOpen *ipConnTracker
	connWG sync.WaitGroup

	metrics       *RelayMetrics
	metricsCtx    context.Context
	metricsCancel context.CancelFunc
	startedUnix   atomic.Int64
	serveOnce     sync.Once

	readQueue *ReaderQueue
}

// NewServer constructs a relay server (handlers and NIP hooks are registered separately).
// relayID may be nil in tests; NIP-29 relay-signed paths require a non-nil identity.
func NewServer(cfg *config.Config, store storage.Store, log zerolog.Logger, relayID *relayidentity.Identity) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("relay: nil config")
	}
	if store == nil {
		return nil, errors.New("relay: nil store")
	}
	mctx, mcancel := context.WithCancel(context.Background())
	s := &Server{
		cfg:           cfg,
		store:         store,
		log:           log,
		relayID:       relayID,
		registry:      NewRegistry(),
		validators:    &ValidatorChain{},
		hooks:         &HookChain{},
		subs:          NewSubscriptionManager(cfg, log),
		limiter:       NewLimiterHub(cfg),
		ipOpen:        newIPConnTracker(),
		metrics:       newRelayMetrics(),
		metricsCtx:    mctx,
		metricsCancel: mcancel,
	}
	s.readQueue = newReaderQueue(s)
	mux := http.NewServeMux()
	mux.Handle("/health", &HealthHandler{Store: store})
	mux.HandleFunc("/", s.handleRoot)
	s.http = &http.Server{
		Handler: mux,
		// Hijacked WebSocket connections manage their own deadlines.
		ReadTimeout:  0,
		WriteTimeout: 0,
	}
	return s, nil
}

// RegisterMessageHandler registers a NIP-01 command handler (used by nips loader).
func (s *Server) RegisterMessageHandler(typ string, h MessageHandler) {
	s.registry.Register(typ, h)
}

// AppendValidator adds to the event validation chain.
func (s *Server) AppendValidator(v EventValidator) {
	s.validators.Append(v)
}

// AppendPostHook adds a post-accept hook with a stable name (logged on hook failure).
func (s *Server) AppendPostHook(name string, h PostStoreHook) {
	s.hooks.Append(name, h)
}

// PrependPostHook adds a hook that runs before hooks registered with AppendPostHook.
func (s *Server) PrependPostHook(name string, h PostStoreHook) {
	s.hooks.Prepend(name, h)
}

// Subscriptions exposes the subscription manager (e.g. for tests).
func (s *Server) Subscriptions() *SubscriptionManager { return s.subs }

// OpenConnections returns the number of active WebSocket relay connections.
func (s *Server) OpenConnections() int64 { return s.open.Load() }

// StartedAtUnix is set when Serve begins (0 before first Serve).
func (s *Server) StartedAtUnix() int64 { return s.startedUnix.Load() }

// Metrics returns wire-level counters and latency samples for the admin API.
func (s *Server) Metrics() *RelayMetrics { return s.metrics }

// Addr returns the bound listener address when serving.
func (s *Server) Addr() string {
	if s.http == nil {
		return ""
	}
	return s.http.Addr
}

// ListenAndServe binds to addr and serves until closed.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve runs the HTTP server on an existing listener (e.g. :0 in tests).
func (s *Server) Serve(ln net.Listener) error {
	s.serveOnce.Do(func() {
		s.startedUnix.Store(time.Now().Unix())
		if s.metrics != nil && s.store != nil {
			s.metrics.StartFlushLoop(s.metricsCtx, s.store, s.cfg.Metrics.RelayBucketRetentionDays, s.log, func() int {
				return s.subs.TotalSubscriptions()
			})
		}
		go s.connAuditSampler()
		go s.idleConnSweeper()
		s.readQueue.start()
	})
	s.http.Addr = ln.Addr().String()
	return s.http.Serve(ln)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions && s.cfg.NIP11.CORSAllowAnyOrigin && !isWebSocketUpgrade(r) {
		writeNIP11CORSPreflightHeaders(w, r)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if isWebSocketUpgrade(r) {
		s.acceptWebSocket(w, r)
		return
	}
	if AcceptsNostrJSON(r) {
		(&NIP11Handler{Cfg: s.cfg}).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Upgrade", "websocket")
	http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
}

func isWebSocketUpgrade(r *http.Request) bool {
	conn := r.Header.Get("Connection")
	up := r.Header.Get("Upgrade")
	return (conn != "" && containsTokenFold(conn, "upgrade")) &&
		stringsFoldEq(up, "websocket")
}

func containsTokenFold(s, tok string) bool {
	// minimal token scan for "upgrade" in Connection header
	for _, part := range splitComma(s) {
		if stringsFoldEq(stringsTrimSpace(part), tok) {
			return true
		}
	}
	return false
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func stringsTrimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[i:j]
}

func stringsFoldEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func (s *Server) acceptWebSocket(w http.ResponseWriter, r *http.Request) {
	if int(s.open.Load()) >= s.cfg.ConnectionLimits.MaxOpen {
		if s.metrics != nil {
			s.metrics.IncRateLimitMaxConnections()
		}
		s.log.Warn().
			Int64("open_connections", s.open.Load()).
			Int("max_open", s.cfg.ConnectionLimits.MaxOpen).
			Str("remote", r.RemoteAddr).
			Msg("connection rejected: global open connection limit reached")
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	peer := clientIP(r)
	if !s.limiter.AllowNewConnection(peer) {
		if s.metrics != nil {
			s.metrics.IncRateLimitNewConnections()
		}
		s.log.Warn().
			Str("peer_ip", peer).
			Str("remote", r.RemoteAddr).
			Int("connections_per_minute_per_ip", s.cfg.ConnectionLimits.ConnectionsPerMinutePerIP).
			Msg("connection rejected: per-IP connection rate limit")
		http.Error(w, "connection rate limit", http.StatusTooManyRequests)
		return
	}

	maxPerIP := s.cfg.ConnectionLimits.MaxOpenPerIP
	openForIP, acquired := s.ipOpen.tryAcquire(peer, maxPerIP)
	if !acquired {
		if s.metrics != nil {
			s.metrics.IncRateLimitPerIPOpen()
		}
		s.log.Warn().
			Str("peer_ip", peer).
			Str("remote", r.RemoteAddr).
			Int("open_for_ip", openForIP).
			Int("max_open_per_ip", maxPerIP).
			Msg("connection rejected: per-IP open connection limit reached")
		http.Error(w, "too many connections from this IP", http.StatusTooManyRequests)
		return
	}

	nc, useFlate, err := s.upgradeConn(w, r)
	if err != nil {
		s.ipOpen.release(peer)
		s.log.Warn().Err(err).Str("remote", r.RemoteAddr).Str("peer_ip", peer).Msg("websocket upgrade failed")
		return
	}

	s.open.Add(1)
	s.connWG.Add(1)
	go s.serveWS(nc, r, peer, useFlate)
}

func (s *Server) upgradeConn(w http.ResponseWriter, r *http.Request) (net.Conn, bool, error) {
	if !s.cfg.WebSocket.CompressionEnabled {
		c, _, _, err := ws.UpgradeHTTP(r, w)
		return c, false, err
	}
	e := &wsflate.Extension{
		Parameters: wsflate.Parameters{
			ServerNoContextTakeover: true,
			ClientNoContextTakeover: true,
		},
	}
	u := ws.HTTPUpgrader{Negotiate: e.Negotiate}
	c, _, _, err := u.Upgrade(r, w)
	if err != nil {
		return nil, false, err
	}
	_, ok := e.Accepted()
	return c, ok, nil
}

func (s *Server) serveWS(nc net.Conn, r *http.Request, resolvedPeerIP string, useFlate bool) {
	defer s.connWG.Done()
	defer s.open.Add(-1)
	defer s.ipOpen.release(resolvedPeerIP)

	_, port, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		port = ""
	}
	remoteAddr := resolvedPeerIP
	if port != "" {
		remoteAddr = net.JoinHostPort(resolvedPeerIP, port)
	}

	id := newConnID()
	log := s.log.With().Str("conn_id", id).Logger()
	ctx, cancel := context.WithCancel(context.Background())
	wsTransport := "plain"
	if useFlate {
		wsTransport = "permessage-deflate"
	}
	c := &Conn{
		ID:          id,
		server:      s,
		peerIP:      resolvedPeerIP,
		remoteAddr:  remoteAddr,
		wsTransport: wsTransport,
		nc:          nc,
		send:        make(chan []byte, 256),
		writerDone:  make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		limiter:     s.limiter.NewConnLimiter(),
		log:         log,
		startedUnix: time.Now().Unix(),
	}
	c.initIdleClock()
	c.log.Info().
		Str("peer_ip", resolvedPeerIP).
		Str("remote_addr", remoteAddr).
		Str("ws_transport", wsTransport).
		Int("open_for_ip", s.ipOpen.openCount(resolvedPeerIP)).
		Int("max_open_per_ip", s.cfg.ConnectionLimits.MaxOpenPerIP).
		Int("idle_no_event_no_sub_seconds", s.cfg.ConnectionLimits.IdleNoEventNoSubSeconds).
		Msg("ws client connected")

	s.conns.Store(id, c)
	defer s.conns.Delete(id)

	s.subs.RegisterSender(id, func(b []byte) bool {
		return c.enqueue(b) == nil
	})

	go c.writeLoop()
	if slices.Contains(s.cfg.NIPs.Enabled, 42) && s.cfg.NIP42.SendChallengeOnConnect {
		_ = nip42EnqueueAuthChallenge(c, s.cfg)
	}
	if useFlate {
		c.readLoopFlate()
	} else {
		c.readLoopPlain()
	}

	c.log.Info().Msg("ws client disconnected")
	s.persistConnAuditSession(c)

	ids := s.subs.UnregisterSender(id)
	for _, sid := range ids {
		_ = c.sendClosed(sid, "connection closed")
	}
	c.initiateShutdown()
	if !c.waitWriterDone(connShutdownWriterWait) {
		c.log.Warn().Dur("wait_seconds", connShutdownWriterWait).
			Msg("ws client write loop did not exit after shutdown; continuing teardown")
	}
}

// Shutdown stops listening and closes active WebSocket connections.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.readQueue != nil {
		s.readQueue.stop()
	}
	if s.metricsCancel != nil {
		s.metricsCancel()
	}
	if s.metrics != nil && s.store != nil {
		_ = s.metrics.FlushOpenMinute(context.Background(), s.store, func() int {
			return s.subs.TotalSubscriptions()
		})
	}
	s.conns.Range(func(_, v any) bool {
		v.(*Conn).initiateShutdown()
		return true
	})
	return s.http.Shutdown(ctx)
}

func peerIP(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

func parseClientIPHeader(v string) (string, bool) {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "", false
	}
	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String(), true
	}
	return "", false
}

func clientIP(r *http.Request) string {
	// Cloudflare Tunnel and other Cloudflare-proxied HTTP traffic set this with the
	// original visitor IP. Prefer it over X-Forwarded-For (Cloudflare recommendation).
	if ip, ok := parseClientIPHeader(r.Header.Get("CF-Connecting-IP")); ok {
		return ip
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.SplitN(xff, ",", 2)[0]
		if ip, ok := parseClientIPHeader(first); ok {
			return ip
		}
	}

	if ip, ok := parseClientIPHeader(r.Header.Get("X-Real-IP")); ok {
		return ip
	}

	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		for _, part := range strings.Split(fwd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "for=") {
				ip := strings.TrimPrefix(part, "for=")
				ip = strings.Trim(ip, "\" \t")
				if parsed, ok := parseClientIPHeader(ip); ok {
					return parsed
				}
				break
			}
		}
	}

	return peerIP(r.RemoteAddr)
}
