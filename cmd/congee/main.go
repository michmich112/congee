package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/michmich112/congee/internal/admin"
	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/relay"
	"github.com/rs/zerolog"
)

func main() {
	tryLoadDotenv()

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

	storeDB, err := db.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("database open failed")
	}
	defer storeDB.Close()

	srv, err := relay.NewServer(cfg, storeDB, log)
	if err != nil {
		log.Fatal().Err(err).Msg("relay server init failed")
	}
	go relay.RunImportedEventFanout(ctx, srv, storeDB, storeDB.EventNotifier, log)
	if err := nips.LoadEnabled(cfg, srv, storeDB, log); err != nil {
		log.Fatal().Err(err).Msg("nips load failed")
	}

	audit.StartRetentionLoop(ctx, storeDB, cfg.Audit.RetentionDays, log)

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
		adminSrv = admin.NewServer(cfg, path, storeDB, srv, log, admin.AdminPassword(), staticDir)
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

// tryLoadDotenv loads ./.env from the process working directory when the file exists.
// Variables already set in the environment are not overridden (same as godotenv default).
// Missing .env is normal (e.g. production); parse/read errors are printed to stderr.
func tryLoadDotenv() {
	const name = ".env"
	st, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		fmt.Fprintf(os.Stderr, "congee: %s: %v\n", name, err)
		return
	}
	if st.IsDir() {
		fmt.Fprintf(os.Stderr, "congee: %s is a directory, skipping\n", name)
		return
	}
	if err := godotenv.Load(name); err != nil {
		fmt.Fprintf(os.Stderr, "congee: loading %s: %v\n", name, err)
	}
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
