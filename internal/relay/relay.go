package relay

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// Server is the Nostr relay HTTP and WebSocket front end.
type Server struct {
	cfg   *config.Config
	store storage.Store
	log   zerolog.Logger

	registry   *Registry
	validators *ValidatorChain
	hooks      *HookChain
	subs       *SubscriptionManager
	limiter    *Hub

	http *http.Server

	open   atomic.Int64
	conns  sync.Map // conn id -> *Conn
	connWG sync.WaitGroup
}

// NewServer constructs a relay server (handlers and NIP hooks are registered separately).
func NewServer(cfg *config.Config, store storage.Store, log zerolog.Logger) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("relay: nil config")
	}
	if store == nil {
		return nil, errors.New("relay: nil store")
	}
	s := &Server{
		cfg:        cfg,
		store:      store,
		log:        log,
		registry:   NewRegistry(),
		validators: &ValidatorChain{},
		hooks:      &HookChain{},
		subs:       NewSubscriptionManager(cfg),
		limiter:    NewLimiterHub(cfg),
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

// AppendPostHook adds a post-accept hook.
func (s *Server) AppendPostHook(h PostStoreHook) {
	s.hooks.Append(h)
}

// Subscriptions exposes the subscription manager (e.g. for tests).
func (s *Server) Subscriptions() *SubscriptionManager { return s.subs }

// OpenConnections returns the number of active WebSocket relay connections.
func (s *Server) OpenConnections() int64 { return s.open.Load() }

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
	s.http.Addr = ln.Addr().String()
	return s.http.Serve(ln)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
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
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}
	peer := peerIP(r.RemoteAddr)
	if !s.limiter.AllowNewConnection(peer) {
		http.Error(w, "connection rate limit", http.StatusTooManyRequests)
		return
	}

	nc, useFlate, err := s.upgradeConn(w, r)
	if err != nil {
		s.log.Debug().Err(err).Str("remote", r.RemoteAddr).Msg("websocket upgrade failed")
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

func (s *Server) serveWS(nc net.Conn, r *http.Request, peerIP string, useFlate bool) {
	defer s.connWG.Done()
	defer s.open.Add(-1)

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
		peerIP:      peerIP,
		remoteAddr:  r.RemoteAddr,
		wsTransport: wsTransport,
		nc:          nc,
		send:        make(chan []byte, 256),
		writerDone:  make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		limiter:     s.limiter.NewConnLimiter(),
		log:         log,
	}
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
	if useFlate {
		c.readLoopFlate()
	} else {
		c.readLoopPlain()
	}

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
