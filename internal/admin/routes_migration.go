package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/postgres"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

var migrationRunning atomic.Bool

type migrationEndpoint struct {
	Type string `json:"type"`
	DSN  string `json:"dsn"`
}

type migrationStartRequest struct {
	Source migrationEndpoint `json:"source"`
	Target migrationEndpoint `json:"target"`
}

// migrationLogConn adds non-secret fields so operators can correlate failures with DSN shape.
// Call on a *zerolog.Event before .Msg(...).
func migrationLogConn(ev *zerolog.Event, role string, typ, dsn string) *zerolog.Event {
	ev = ev.Str(role+"_type", typ).Bool(role+"_dsn_empty", dsn == "")
	switch typ {
	case "sqlite":
		if dsn != "" {
			ev = ev.Str(role+"_dsn_basename", filepath.Base(dsn)).Int(role+"_dsn_len", len(dsn))
		}
	case "postgres":
		if u, err := url.Parse(dsn); err == nil && u.Host != "" {
			ev = ev.Str(role+"_dsn_scheme", u.Scheme).Str(role+"_dsn_host", u.Host)
		} else if dsn != "" {
			ev = ev.Bool(role+"_dsn_parse_ok", false).Int(role+"_dsn_len", len(dsn))
		}
	}
	return ev
}

func openMigrationSource(ctx context.Context, typ, dsn string, log zerolog.Logger) (storage.MigrationSource, func(), error) {
	switch typ {
	case "sqlite":
		if dsn == "" {
			return nil, nil, errors.New("sqlite dsn is required")
		}
		st, err := sqlite.Open(ctx, dsn, nil, log)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	case "postgres":
		if dsn == "" {
			return nil, nil, errors.New("postgres dsn is required")
		}
		st, err := postgres.Open(ctx, dsn, log)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported database type %q (use sqlite or postgres)", typ)
	}
}

// applyPostMigrationDatabaseConfig updates database.type and database.dsn in the JSON config
// to match the migration target, records a changelog row on dst (the new canonical store), and
// returns whether the running relay must restart to pick up the new file.
func applyPostMigrationDatabaseConfig(ctx context.Context, cfgPath string, cfgMu *sync.Mutex, dst storage.MigrationSource, target migrationEndpoint, relayID *relayidentity.Identity) (restartNeeded bool, err error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	prev, _ := os.ReadFile(cfgPath)
	cfg, err := config.LoadJSON(cfgPath)
	if err != nil {
		return false, fmt.Errorf("load config: %w", err)
	}

	dbType := strings.TrimSpace(strings.ToLower(target.Type))
	switch dbType {
	case "sqlite", "postgres":
	default:
		return false, fmt.Errorf("unsupported target type %q", target.Type)
	}
	cfg.Database.Type = dbType
	cfg.Database.DSN = strings.TrimSpace(target.DSN)
	if err := cfg.Validate(); err != nil {
		return false, err
	}
	if relayID != nil {
		if p := strings.TrimSpace(cfg.NIP11.PubKey); p != "" && !strings.EqualFold(p, relayID.PubKeyHex()) {
			return false, fmt.Errorf("nip11.pubkey must match relay identity %s or be empty", relayID.PubKeyHex())
		}
	}

	needRestart := configRestartNeeded(prev, cfg)
	if err := config.WriteConfigAtomic(cfgPath, cfg); err != nil {
		return false, err
	}
	dbJSON, err := json.Marshal(cfg.Database)
	if err != nil {
		return false, err
	}
	diff := "database=" + string(dbJSON)
	if len(prev) > 0 {
		diff = "previous_bytes=" + strconv.Itoa(len(prev)) + "\n" + diff
	}
	summary := "POST /api/migration/start: database set to " + dbType
	if err := config.SaveConfigChange(ctx, dst, summary, diff); err != nil {
		return false, fmt.Errorf("changelog on target: %w", err)
	}
	return needRestart, nil
}

func handleMigrationStart(log zerolog.Logger, cfgPath string, cfgMu *sync.Mutex, scheduleRestart func(), relayID *relayidentity.Identity) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !migrationRunning.CompareAndSwap(false, true) {
			http.Error(w, "migration already running", http.StatusConflict)
			return
		}
		defer migrationRunning.Store(false)

		log.Debug().Str("handler", "migration_start").Msg("received migration start request")

		var req migrationStartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn().Err(err).Str("handler", "migration_start").Msg("migration rejected: invalid json body")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Source.Type == "" || req.Target.Type == "" {
			log.Warn().
				Str("handler", "migration_start").
				Str("source_type", req.Source.Type).
				Str("target_type", req.Target.Type).
				Msg("migration rejected: source.type and/or target.type missing")
			http.Error(w, "source.type and target.type are required", http.StatusBadRequest)
			return
		}

		l := log.With().Str("handler", "migration_start").Logger()
		ctx := r.Context()
		migrationLogConn(l.Debug(), "source", req.Source.Type, req.Source.DSN).Msg("opening migration source")

		src, closeSrc, err := openMigrationSource(ctx, req.Source.Type, req.Source.DSN, l)
		if err != nil {
			migrationLogConn(log.Warn(), "source", req.Source.Type, req.Source.DSN).
				Err(err).Str("phase", "open_source").Msg("migration rejected: open source failed")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer closeSrc()

		migrationLogConn(l.Debug(), "target", req.Target.Type, req.Target.DSN).Msg("opening migration target")

		dst, closeDst, err := openMigrationSource(ctx, req.Target.Type, req.Target.DSN, l)
		if err != nil {
			migrationLogConn(log.Warn(), "target", req.Target.Type, req.Target.DSN).
				Err(err).Str("phase", "open_target").Msg("migration rejected: open target failed")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer closeDst()

		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		send := func(name string, v any) {
			b, err := json.Marshal(v)
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
			fl.Flush()
		}

		send("ready", map[string]string{"status": "started"})
		l.Debug().Msg("sse started; running storage.Migrate")

		sum, err := storage.Migrate(ctx, src, dst, func(p storage.MigrationProgress) {
			l.Trace().
				Float64("percent", p.Percent).
				Str("message", p.Message).
				Msg("migration progress")
			send("progress", p)
		}, func(step string) {
			l.Debug().Str("milestone", step).Msg("migration milestone")
		})
		if err != nil {
			l.Warn().Err(err).Msg("migration copy failed")
			send("error", map[string]any{
				"message": err.Error(),
				"summary": nil,
			})
			return
		}
		l.Debug().Msg("migration finished ok")

		restartNeeded, cfgErr := applyPostMigrationDatabaseConfig(ctx, cfgPath, cfgMu, dst, req.Target, relayID)
		if cfgErr != nil {
			l.Warn().Err(cfgErr).Msg("migration copy ok but config update failed")
		} else if restartNeeded && scheduleRestart != nil {
			go scheduleRestartSoon(scheduleRestart)
		}

		done := map[string]any{
			"status":               "ok",
			"summary":              sum,
			"config_updated":       cfgErr == nil,
			"restart_required":     restartNeeded && cfgErr == nil,
			"restarting":           restartNeeded && cfgErr == nil && scheduleRestart != nil,
			"target_type":          strings.TrimSpace(strings.ToLower(req.Target.Type)),
			"target_dsn_nonsecret": migrationTargetDSNForUI(req.Target.Type, req.Target.DSN),
		}
		if cfgErr != nil {
			done["config_error"] = cfgErr.Error()
		}
		send("done", done)
	}
}

// migrationTargetDSNForUI returns a short non-secret hint for the admin UI (full DSN stays server-side in config).
func migrationTargetDSNForUI(typ, dsn string) string {
	dsn = strings.TrimSpace(dsn)
	switch strings.TrimSpace(strings.ToLower(typ)) {
	case "sqlite":
		if dsn == "" {
			return ""
		}
		return filepath.Base(dsn)
	case "postgres":
		if u, err := url.Parse(dsn); err == nil && u.Host != "" {
			return u.Host + u.Path
		}
		return ""
	default:
		return ""
	}
}
