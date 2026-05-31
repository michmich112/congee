package relay

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
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
	connWG sync.WaitGroup

	metrics       *RelayMetrics
	metricsCtx    context.Context
	metricsCancel context.CancelFunc
	startedUnix   atomic.Int64
	serveOnce     sync.Once

	plugins PluginRunner
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
		metrics:       newRelayMetrics(),
		metricsCtx:    mctx,
		metricsCancel: mcancel,
	}
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
		(&NIP11Handler{Cfg: s.cfg, Server: s}).ServeHTTP(w, r)
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
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	peer := clientIP(r)
	if !s.limiter.AllowNewConnection(peer) {
		if s.metrics != nil {
			s.metrics.IncRateLimitNewConnections()
		}
		http.Error(w, "connection rate limit", http.StatusTooManyRequests)
		return
	}

	nc, useFlate, err := s.upgradeConn(w, r)
	if err != nil {
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
	c.log.Info().Str("peer_ip", resolvedPeerIP).Str("ws_transport", wsTransport).Msg("ws client connected")
	defer cancel()

	s.conns.Store(id, c)
	defer s.conns.Delete(id)

	s.subs.RegisterSender(id, func(b []byte) bool {
		select {
		case c.send <- b:
			return true
		case <-ctx.Done():
			return false
		default:
			return false
		}
	})

	go c.writeLoop()
	if s.cfg.NIP42.Enabled && s.cfg.NIP42.SendChallengeOnConnect {
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
		b, err := nostr.MarshalRelayClosed(sid, "connection closed")
		if err != nil {
			continue
		}
		select {
		case c.send <- b:
		default:
		}
	}
	close(c.send)
	<-c.writerDone
	_ = nc.Close()
}

// Shutdown stops listening and closes active WebSocket connections.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.metricsCancel != nil {
		s.metricsCancel()
	}
	if s.metrics != nil && s.store != nil {
		_ = s.metrics.FlushOpenMinute(context.Background(), s.store, func() int {
			return s.subs.TotalSubscriptions()
		})
	}
	s.conns.Range(func(_, v any) bool {
		c := v.(*Conn)
		c.cancel()
		_ = c.nc.Close()
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.SplitN(xff, ",", 2)[0]
		trimmed := strings.TrimSpace(first)
		if net.ParseIP(trimmed) != nil {
			return trimmed
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		trimmed := strings.TrimSpace(xri)
		if net.ParseIP(trimmed) != nil {
			return trimmed
		}
	}

	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		for _, part := range strings.Split(fwd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "for=") {
				ip := strings.TrimPrefix(part, "for=")
				ip = strings.Trim(ip, "\" \t")
				if net.ParseIP(ip) != nil {
					return ip
				}
				break
			}
		}
	}

	return peerIP(r.RemoteAddr)
}
