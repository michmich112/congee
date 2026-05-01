package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sync/atomic"

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

func handleMigrationStart(log zerolog.Logger) http.HandlerFunc {
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

		err = storage.Migrate(ctx, src, dst, func(p storage.MigrationProgress) {
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
			send("error", map[string]string{"message": err.Error()})
			return
		}
		l.Debug().Msg("migration finished ok")
		send("done", map[string]string{"status": "ok"})
	}
}
