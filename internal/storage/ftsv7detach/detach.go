// Package ftsv7detach removes the SQLite FTS5 shadow table from events
// databases created before schema version 8, and snapshots the meta database
// before its first tursogo open.
//
// Delete this package and the go-libsql dependency when a release no longer
// opens events files with user_version below 8. Nothing else may call it.
// turso.Open and sqlitemeta.Open are the only callers.
package ftsv7detach

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
	"golang.org/x/sys/unix"
)

const backupSuffix = ".pre-v8.bak"

// eventAggregates are the event-row invariants the FTS5 detach must preserve.
type eventAggregates struct {
	Events       int64
	Tags         int64
	ContentBytes int64
}

func (a eventAggregates) mismatch(b eventAggregates) error {
	if a == b {
		return nil
	}
	return fmt.Errorf("ftsv7detach: event aggregates changed events %d->%d tags %d->%d content_bytes %d->%d",
		a.Events, b.Events, a.Tags, b.Tags, a.ContentBytes, b.ContentBytes)
}

// verifyEventsAggregates is the detach check. Tests replace it to force a rollback.
var verifyEventsAggregates = func(before, after eventAggregates) error {
	return before.mismatch(after)
}

// MetaCounts is a per-table row count taken before the meta file is opened with tursogo.
type MetaCounts struct {
	Tables map[string]int64 `json:"tables"`
}

// PrepareEvents drops FTS5 search objects when they are present.
// A missing file or a file without FTS5 is a no-op. Event and tag rows are not modified.
func PrepareEvents(ctx context.Context, path string, log zerolog.Logger) error {
	if path == "" {
		return errors.New("ftsv7detach: empty events path")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ftsv7detach: stat events: %w", err)
	}
	has, err := HasEventFTS5(ctx, path)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return detachEventsFTS5(ctx, path, log)
}

// HasEventFTS5 reports whether the file's schema contains the FTS5 search table or its triggers.
// The probe uses a read-write libSQL connection: mode=ro stays locked after BEGIN IMMEDIATE
// in the same process, which is how the detach transaction closes.
func HasEventFTS5(ctx context.Context, path string) (bool, error) {
	db, err := openLibsqlRW(ctx, path)
	if err != nil {
		if libsqlSeesTursoIndex(err) {
			return false, nil
		}
		return false, err
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE name = 'event_fts'
		   OR name IN ('events_ai_fts', 'events_au_fts', 'events_ad_fts')
		   OR (sql LIKE '%fts5%' AND type IN ('table', 'trigger'))`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("ftsv7detach: probe fts5: %w", err)
	}
	return n > 0, nil
}

// libsqlSeesTursoIndex reports that go-libsql refused the file because it already
// contains a Turso full-text index. Those files are past the FTS5 upgrade.
func libsqlSeesTursoIndex(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "malformed database schema") && strings.Contains(msg, "USING")
}

func detachEventsFTS5(ctx context.Context, path string, log zerolog.Logger) error {
	if err := checkpointWAL(ctx, path); err != nil {
		return err
	}
	bak := path + backupSuffix
	if _, err := os.Stat(bak); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("ftsv7detach: stat backup: %w", err)
		}
		if err := backupFile(path, bak); err != nil {
			return err
		}
		log.Info().Str("backup", bak).Msg("events database backup written before fts5 detach")
	}
	db, err := openLibsqlRW(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("ftsv7detach: begin: %w", err)
	}
	before, err := readEventAggregates(ctx, db)
	if err != nil {
		_, _ = db.ExecContext(ctx, `ROLLBACK`)
		return err
	}
	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS events_ai_fts`,
		`DROP TRIGGER IF EXISTS events_au_fts`,
		`DROP TRIGGER IF EXISTS events_ad_fts`,
		`DROP TABLE IF EXISTS event_fts`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_, _ = db.ExecContext(ctx, `ROLLBACK`)
			return fmt.Errorf("ftsv7detach: %s: %w", stmt, err)
		}
	}
	after, err := readEventAggregates(ctx, db)
	if err != nil {
		_, _ = db.ExecContext(ctx, `ROLLBACK`)
		return err
	}
	if err := verifyEventsAggregates(before, after); err != nil {
		_, _ = db.ExecContext(ctx, `ROLLBACK`)
		return err
	}
	if _, err := db.ExecContext(ctx, `COMMIT`); err != nil {
		return fmt.Errorf("ftsv7detach: commit: %w", err)
	}
	if err := checkpointConn(ctx, db); err != nil {
		return err
	}
	log.Info().Str("backup", bak).Msg("fts5 search index removed; event rows unchanged")
	return nil
}

func readEventAggregates(ctx context.Context, db *sql.DB) (eventAggregates, error) {
	var a eventAggregates
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&a.Events); err != nil {
		return a, fmt.Errorf("ftsv7detach: count events: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_tags`).Scan(&a.Tags); err != nil {
		return a, fmt.Errorf("ftsv7detach: count event_tags: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(content)), 0) FROM events`).Scan(&a.ContentBytes); err != nil {
		return a, fmt.Errorf("ftsv7detach: sum content: %w", err)
	}
	return a, nil
}

// PrepareMeta copies an existing meta database once and returns the row counts to confirm after tursogo opens it.
// skip is true when the file is absent or this upgrade already succeeded.
func PrepareMeta(ctx context.Context, path string, log zerolog.Logger) (MetaCounts, bool, error) {
	var zero MetaCounts
	if path == "" {
		return zero, false, errors.New("ftsv7detach: empty meta path")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return zero, true, nil
		}
		return zero, false, fmt.Errorf("ftsv7detach: stat meta: %w", err)
	}
	okPath := path + ".pre-v8.ok"
	if _, err := os.Stat(okPath); err == nil {
		return zero, true, nil
	} else if !os.IsNotExist(err) {
		return zero, false, fmt.Errorf("ftsv7detach: stat meta ok: %w", err)
	}
	countsPath := path + ".pre-v8.counts"
	bak := path + backupSuffix
	if _, err := os.Stat(bak); err != nil {
		if !os.IsNotExist(err) {
			return zero, false, fmt.Errorf("ftsv7detach: stat meta backup: %w", err)
		}
		if err := checkpointWAL(ctx, path); err != nil {
			return zero, false, err
		}
		if err := backupFile(path, bak); err != nil {
			return zero, false, err
		}
		counts, err := readMetaCounts(ctx, path)
		if err != nil {
			return zero, false, err
		}
		raw, err := json.Marshal(counts)
		if err != nil {
			return zero, false, err
		}
		if err := writeSynced(countsPath, raw); err != nil {
			return zero, false, err
		}
		log.Info().Str("backup", bak).Msg("meta database backup written before tursogo open")
		return counts, false, nil
	}
	raw, err := os.ReadFile(countsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return zero, false, fmt.Errorf("ftsv7detach: read meta counts: %w", err)
		}
		counts, err := readMetaCounts(ctx, bak)
		if err != nil {
			return zero, false, err
		}
		encoded, err := json.Marshal(counts)
		if err != nil {
			return zero, false, err
		}
		if err := writeSynced(countsPath, encoded); err != nil {
			return zero, false, err
		}
		return counts, false, nil
	}
	var counts MetaCounts
	if err := json.Unmarshal(raw, &counts); err != nil {
		return zero, false, fmt.Errorf("ftsv7detach: decode meta counts: %w", err)
	}
	return counts, false, nil
}

// ConfirmMeta checks live meta row counts against the pre-open snapshot and records success.
func ConfirmMeta(ctx context.Context, db *sql.DB, path string, counts MetaCounts, log zerolog.Logger) error {
	for name, want := range counts.Tables {
		var got int64
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(name))
		if err := db.QueryRowContext(ctx, q).Scan(&got); err != nil {
			return fmt.Errorf("ftsv7detach: count meta %s: %w", name, err)
		}
		if got != want {
			return fmt.Errorf("ftsv7detach: meta %s rows %d want %d", name, got, want)
		}
	}
	okPath := path + ".pre-v8.ok"
	if err := writeSynced(okPath, []byte("ok\n")); err != nil {
		return err
	}
	log.Info().Str("backup", path+backupSuffix).Msg("meta database row counts unchanged after tursogo open")
	return nil
}

func readMetaCounts(ctx context.Context, path string) (MetaCounts, error) {
	db, err := sql.Open("libsql", "file:"+path+"?mode=ro")
	if err != nil {
		return MetaCounts{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	names := []string{"audit_log", "config_changelog", "relay_metric_buckets", "ws_connection_sessions"}
	out := MetaCounts{Tables: map[string]int64{}}
	for _, name := range names {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&n); err != nil {
			return MetaCounts{}, err
		}
		if n == 0 {
			continue
		}
		var rows int64
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(name))
		if err := db.QueryRowContext(ctx, q).Scan(&rows); err != nil {
			return MetaCounts{}, fmt.Errorf("ftsv7detach: count %s: %w", name, err)
		}
		out.Tables[name] = rows
	}
	return out, nil
}

func quoteIdent(name string) string {
	return `"` + name + `"`
}

func checkpointWAL(ctx context.Context, path string) error {
	db, err := openLibsqlRW(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := requireWAL(ctx, db); err != nil {
		return err
	}
	return checkpointConn(ctx, db)
}

func requireWAL(ctx context.Context, db *sql.DB) error {
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&mode); err != nil {
		return fmt.Errorf("ftsv7detach: journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("ftsv7detach: journal_mode %q", mode)
	}
	return nil
}

func checkpointConn(ctx context.Context, db *sql.DB) error {
	var a, b, c int
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&a, &b, &c); err != nil {
		return fmt.Errorf("ftsv7detach: checkpoint: %w", err)
	}
	return nil
}

func openLibsqlRW(ctx context.Context, path string) (*sql.DB, error) {
	sqldb, _, err := sqlitewriter.OpenLibsqlHandles(ctx, path, zerolog.Nop())
	if err != nil {
		return nil, fmt.Errorf("ftsv7detach: open libsql: %w", err)
	}
	return sqldb, nil
}

func backupFile(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	dir := filepath.Dir(src)
	free, err := freeBytes(dir)
	if err != nil {
		return err
	}
	if free < uint64(st.Size()) {
		return fmt.Errorf("ftsv7detach: not enough free space to back up %s", src)
	}
	tmp := dst + ".tmp"
	_ = os.Remove(tmp)
	if err := copySynced(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ftsv7detach: rename backup: %w", err)
	}
	return fsyncDir(dir)
}

func copySynced(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func writeSynced(path string, data []byte) error {
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return fsyncDir(filepath.Dir(path))
}

func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func freeBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("ftsv7detach: statfs: %w", err)
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
