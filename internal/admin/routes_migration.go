package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/postgres"
	"github.com/michmich112/congee/internal/storage/sqlite"
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

func openMigrationSource(ctx context.Context, typ, dsn string) (storage.MigrationSource, func(), error) {
	switch typ {
	case "sqlite":
		if dsn == "" {
			return nil, nil, errors.New("sqlite dsn is required")
		}
		st, err := sqlite.Open(ctx, dsn, nil)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	case "postgres":
		if dsn == "" {
			return nil, nil, errors.New("postgres dsn is required")
		}
		st, err := postgres.Open(ctx, dsn)
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported database type %q (use sqlite or postgres)", typ)
	}
}

func handleMigrationStart() http.HandlerFunc {
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

		var req migrationStartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Source.Type == "" || req.Target.Type == "" {
			http.Error(w, "source.type and target.type are required", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		src, closeSrc, err := openMigrationSource(ctx, req.Source.Type, req.Source.DSN)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer closeSrc()

		dst, closeDst, err := openMigrationSource(ctx, req.Target.Type, req.Target.DSN)
		if err != nil {
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

		err = storage.Migrate(ctx, src, dst, func(p storage.MigrationProgress) {
			send("progress", p)
		})
		if err != nil {
			send("error", map[string]string{"message": err.Error()})
			return
		}
		send("done", map[string]string{"status": "ok"})
	}
}
