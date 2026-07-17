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
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

var migrationRunning atomic.Bool

type migrationEndpoint struct {
	Type string `json:"type"`
	DSN  string `json:"dsn"`
}

type migrationStartRequest struct {
	Source            migrationEndpoint `json:"source"`
	Target            migrationEndpoint `json:"target"`
	MakeTargetPrimary bool              `json:"make_target_primary"`
}

type migrationTargetPreflightRequest struct {
	Target migrationEndpoint `json:"target"`
}

func handleMigrationTargetPreflight(log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req migrationTargetPreflightRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn().Err(err).Str("handler", "migration_target_preflight").Msg("invalid json body")
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Target.Type) == "" {
			log.Warn().Str("handler", "migration_target_preflight").Msg("target.type missing")
			http.Error(w, "target.type is required", http.StatusBadRequest)
			return
		}

		l := log.With().Str("handler", "migration_target_preflight").Logger()
		ctx := r.Context()
		migrationLogConn(l.Debug(), "target", req.Target.Type, req.Target.DSN).Msg("migration target preflight")

		var out storage.MigrationTargetPreflight
		switch migrationCanonicalDBType(req.Target.Type) {
		case "postgres":
			out = postgres.PreflightMigrationTarget(ctx, req.Target.DSN, l)
		case "sqlite":
			out = sqlite.PreflightMigrationTarget(ctx, req.Target.DSN, l)
		case "turso":
			out = turso.PreflightMigrationTarget(ctx, req.Target.DSN, l)
		default:
			out = storage.MigrationTargetPreflight{
				Status:          storage.MigrationPreflightUnreadable,
				ExpectedVersion: postgres.CurrentSchemaVersion(),
				Detail:          fmt.Sprintf("unsupported target type %q (use sqlite, turso, or postgres)", req.Target.Type),
			}
		}
		if out.Status == "" {
			out.Status = storage.MigrationPreflightUnreadable
			if out.Detail == "" {
				out.Detail = "preflight returned empty status"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			log.Warn().Err(err).Str("handler", "migration_target_preflight").Msg("encode response failed")
		}
	}
}

// migrationLogConn adds non-secret fields so operators can correlate failures with DSN shape.
// Call on a *zerolog.Event before .Msg(...).
func migrationLogConn(ev *zerolog.Event, role string, typ, dsn string) *zerolog.Event {
	ev = ev.Str(role+"_type", typ).Bool(role+"_dsn_empty", dsn == "")
	switch typ {
	case "sqlite", "turso":
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

// migrationCanonicalDBType maps config database.type (including empty) to a canonical backend id.
func migrationCanonicalDBType(typ string) string {
	switch strings.TrimSpace(strings.ToLower(typ)) {
	case "postgres":
		return "postgres"
	case "sqlite":
		return "sqlite"
	case "", "turso":
		return "turso"
	default:
		return "sqlite"
	}
}

// migrationSourceMatchesConfig reports whether the client source matches the relay JSON config database.
func migrationSourceMatchesConfig(cfg *config.Config, src migrationEndpoint) bool {
	if cfg == nil {
		return false
	}
	return migrationCanonicalDBType(src.Type) == migrationCanonicalDBType(cfg.Database.Type) &&
		strings.TrimSpace(src.DSN) == strings.TrimSpace(cfg.Database.DSN)
}

func openMigrationSource(ctx context.Context, dbType, dsn, congeeInstanceID string, log zerolog.Logger) (storage.MigrationSource, func(), error) {
	switch migrationCanonicalDBType(dbType) {
	case "sqlite":
		if dsn == "" {
			return nil, nil, errors.New("sqlite dsn is required")
		}
		st, err := sqlite.Open(ctx, dsn, nil, log)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	case "turso":
		if dsn == "" {
			return nil, nil, errors.New("turso dsn is required")
		}
		st, err := turso.Open(ctx, dsn, nil, log)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	case "postgres":
		if dsn == "" {
			return nil, nil, errors.New("postgres dsn is required")
		}
		st, err := postgres.Open(ctx, dsn, congeeInstanceID, log)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported database type %q (use sqlite, turso, or postgres)", dbType)
	}
}

// applyPostMigrationDatabaseConfig updates database.type and database.dsn in the JSON config
// to match the migration target, records a changelog row on the running relay meta store, and
// returns whether the running relay must restart to pick up the new file.
func applyPostMigrationDatabaseConfig(ctx context.Context, cfgPath string, cfgMu *sync.Mutex, meta storage.MetaStore, target migrationEndpoint, relayID *relayidentity.Identity) (restartNeeded bool, err error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	prev, _ := os.ReadFile(cfgPath)
	cfg, err := config.LoadJSON(cfgPath)
	if err != nil {
		return false, fmt.Errorf("load config: %w", err)
	}

	dbType := strings.TrimSpace(strings.ToLower(target.Type))
	switch dbType {
	case "sqlite", "postgres", "turso":
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
	if err := config.SaveConfigChange(ctx, meta, summary, diff); err != nil {
		return false, fmt.Errorf("changelog on meta store: %w", err)
	}
	return needRestart, nil
}

func handleMigrationStart(log zerolog.Logger, cfgPath string, cfgMu *sync.Mutex, meta storage.MetaStore, scheduleRestart func(), relayID *relayidentity.Identity) http.HandlerFunc {
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
		if req.Target.Type == "" {
			log.Warn().
				Str("handler", "migration_start").
				Str("target_type", req.Target.Type).
				Msg("migration rejected: target.type missing")
			http.Error(w, "target.type is required", http.StatusBadRequest)
			return
		}

		cfg, err := config.LoadJSON(cfgPath)
		if err != nil {
			log.Warn().Err(err).Str("handler", "migration_start").Msg("migration rejected: load config failed")
			http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !migrationSourceMatchesConfig(cfg, req.Source) {
			log.Warn().
				Str("handler", "migration_start").
				Str("source_type", req.Source.Type).
				Str("config_type", cfg.Database.Type).
				Bool("source_dsn_match", strings.TrimSpace(req.Source.DSN) == strings.TrimSpace(cfg.Database.DSN)).
				Msg("migration rejected: source does not match configured database")
			http.Error(w, "source must match the relay database in config", http.StatusBadRequest)
			return
		}

		l := log.With().Str("handler", "migration_start").Logger()
		ctx := r.Context()
		migrationLogConn(l.Debug(), "source", req.Source.Type, req.Source.DSN).Msg("opening migration source")

		congeeInstanceID := config.ResolveRelayInstance(cfg).EffectiveID

		srcType := migrationCanonicalDBType(req.Source.Type)
		tgtType := migrationCanonicalDBType(req.Target.Type)

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

		var sum storage.MigrationSummary
		var migErr error

		if srcType == "sqlite" && tgtType == "turso" {
			l.Debug().Msg("sqlite->turso native migration via VACUUM INTO")
			send("progress", storage.MigrationProgress{Percent: 5, Message: "copying database file (VACUUM INTO)"})
			sum, migErr = storage.MigrateSQLiteToTursoNative(ctx, req.Source.DSN, req.Target.DSN)
			if migErr == nil {
				send("progress", storage.MigrationProgress{Percent: 100, Message: "native copy complete"})
			}
		} else {
			migrationLogConn(l.Debug(), "source", req.Source.Type, req.Source.DSN).Msg("opening migration source")

			src, closeSrc, openErr := openMigrationSource(ctx, req.Source.Type, req.Source.DSN, congeeInstanceID, l)
			if openErr != nil {
				migrationLogConn(log.Warn(), "source", req.Source.Type, req.Source.DSN).
					Err(openErr).Str("phase", "open_source").Msg("migration rejected: open source failed")
				http.Error(w, openErr.Error(), http.StatusBadRequest)
				return
			}
			defer closeSrc()

			migrationLogConn(l.Debug(), "target", req.Target.Type, req.Target.DSN).Msg("opening migration target")

			dst, closeDst, openErr := openMigrationSource(ctx, req.Target.Type, req.Target.DSN, congeeInstanceID, l)
			if openErr != nil {
				migrationLogConn(log.Warn(), "target", req.Target.Type, req.Target.DSN).
					Err(openErr).Str("phase", "open_target").Msg("migration rejected: open target failed")
				http.Error(w, openErr.Error(), http.StatusBadRequest)
				return
			}
			defer closeDst()

			l.Debug().Msg("sse started; running storage.Migrate")

			sum, migErr = storage.Migrate(ctx, src, dst, func(p storage.MigrationProgress) {
				l.Trace().
					Float64("percent", p.Percent).
					Str("message", p.Message).
					Msg("migration progress")
				send("progress", p)
			}, func(step string) {
				l.Debug().Str("milestone", step).Msg("migration milestone")
			})
		}
		if migErr != nil {
			l.Warn().Err(migErr).Msg("migration copy failed")
			send("error", map[string]any{
				"message": migErr.Error(),
				"summary": nil,
			})
			return
		}
		l.Debug().Msg("migration finished ok")

		var restartNeeded bool
		var cfgErr error
		if req.MakeTargetPrimary {
			l.Debug().Msg("make_target_primary: updating config file to target database")
			restartNeeded, cfgErr = applyPostMigrationDatabaseConfig(ctx, cfgPath, cfgMu, meta, req.Target, relayID)
			if cfgErr != nil {
				l.Warn().Err(cfgErr).Msg("migration copy ok but config update failed")
			} else if restartNeeded && scheduleRestart != nil {
				go scheduleRestartSoon(scheduleRestart)
			}
		} else {
			l.Debug().Msg("make_target_primary false: skipping config file update")
		}

		done := map[string]any{
			"status":               "ok",
			"summary":              sum,
			"make_target_primary":  req.MakeTargetPrimary,
			"config_updated":       req.MakeTargetPrimary && cfgErr == nil,
			"restart_required":     req.MakeTargetPrimary && restartNeeded && cfgErr == nil,
			"restarting":           req.MakeTargetPrimary && restartNeeded && cfgErr == nil && scheduleRestart != nil,
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
	case "sqlite", "turso":
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
