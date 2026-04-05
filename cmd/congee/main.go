package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/michmich112/congee/internal/admin"
	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func main() {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "./config.json"
	}
	cfg, err := config.Load(path)
	if err != nil {
		panic("config: " + err.Error())
	}
	log := setupLogger(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.Database.Type != "" && cfg.Database.Type != "sqlite" {
		log.Fatal().Str("type", cfg.Database.Type).Msg("unsupported database.type (phase 4: sqlite only)")
	}
	store, err := sqlite.Open(ctx, cfg.Database.DSN, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("sqlite open failed")
	}
	defer store.Close()

	srv, err := relay.NewServer(cfg, store, log)
	if err != nil {
		log.Fatal().Err(err).Msg("relay server init failed")
	}
	if err := nips.LoadEnabled(cfg, srv, store, log); err != nil {
		log.Fatal().Err(err).Msg("nips load failed")
	}

	audit.StartRetentionLoop(ctx, store, cfg.Audit.RetentionDays, log)

	addr := relayListenAddr(cfg)
	go func() {
		log.Info().Str("addr", addr).Msg("relay listening")
		if err := srv.ListenAndServe(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("relay server stopped")
			cancel()
		}
	}()

	var adminSrv *admin.Server
	if admin.Enabled() {
		staticDir := filepath.Join("web", "admin", "build")
		adminSrv = admin.NewServer(cfg, path, store, srv, log, admin.AdminPassword(), staticDir)
		go func() {
			if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("admin server stopped")
				cancel()
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info().Msg("shutdown signal")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("relay shutdown error")
	}
	if adminSrv != nil {
		if err := adminSrv.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("admin shutdown error")
		}
	}
	cancel()
	log.Info().Msg("bye")
}

func relayListenAddr(cfg *config.Config) string {
	if cfg.Relay.Port <= 0 {
		return ":3334"
	}
	return ":" + strconv.Itoa(cfg.Relay.Port)
}

func setupLogger(cfg *config.Config) zerolog.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	var out io.Writer = os.Stderr
	if cfg.Logging.Format == "console" {
		out = zerolog.ConsoleWriter{Out: os.Stderr}
	}
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	return zerolog.New(out).Level(level).With().Timestamp().Logger()
}
