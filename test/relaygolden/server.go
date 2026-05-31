package relaygolden

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// Server wraps a running test relay listening on localhost.
type Server struct {
	WSURL string
	HTTP  string

	srv *relay.Server
	ln  net.Listener
	st  storage.Store
}

// Start boots a relay with the given config and store. Caller must call Close.
func Start(cfg *config.Config, st storage.Store, log zerolog.Logger) (*Server, error) {
	if cfg == nil || st == nil {
		return nil, fmt.Errorf("relaygolden: nil config or store")
	}
	if log.GetLevel() == zerolog.NoLevel {
		log = zerolog.Nop()
	}
	srv, err := relay.NewServer(cfg, st, log, nil)
	if err != nil {
		return nil, err
	}
	if err := nips.LoadEnabled(cfg, srv, st, log); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() { _ = srv.Serve(ln) }()
	addr := ln.Addr().(*net.TCPAddr)
	time.Sleep(20 * time.Millisecond)
	return &Server{
		WSURL: fmt.Sprintf("ws://127.0.0.1:%d/", addr.Port),
		HTTP:  fmt.Sprintf("http://127.0.0.1:%d", addr.Port),
		srv:   srv,
		ln:    ln,
		st:    st,
	}, nil
}

// StartWithIdentity is like Start but loads relay identity for NIP-42/NIP-29 paths.
func StartWithIdentity(cfg *config.Config, st storage.Store, log zerolog.Logger, secretsPath string) (*Server, error) {
	rid, err := relayidentity.Load(secretsPath)
	if err != nil {
		return nil, err
	}
	if err := relayidentity.ReconcileNIP11PubKey(cfg, rid); err != nil {
		return nil, err
	}
	if log.GetLevel() == zerolog.NoLevel {
		log = zerolog.Nop()
	}
	srv, err := relay.NewServer(cfg, st, log, rid)
	if err != nil {
		return nil, err
	}
	if err := nips.LoadEnabled(cfg, srv, st, log); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() { _ = srv.Serve(ln) }()
	addr := ln.Addr().(*net.TCPAddr)
	time.Sleep(20 * time.Millisecond)
	return &Server{
		WSURL: fmt.Sprintf("ws://127.0.0.1:%d/", addr.Port),
		HTTP:  fmt.Sprintf("http://127.0.0.1:%d", addr.Port),
		srv:   srv,
		ln:    ln,
		st:    st,
	}, nil
}

// Close shuts down the relay and listener.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if s.srv != nil {
		_ = s.srv.Shutdown(ctx)
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
	return nil
}
