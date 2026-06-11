package db

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/michmich112/congee/internal/config"
)

// ResolveMetaDSN returns the SQLite path for operational metadata.
// Precedence: database.meta_dsn, then sibling congee-meta.db beside the events DSN,
// or CONGEE_DATA_DIR/congee-meta.db when events DSN is under that directory.
// For PostgreSQL database.type, defaults to ./congee-meta.db (local meta sidecar).
func ResolveMetaDSN(sec config.DatabaseSection) string {
	if p := strings.TrimSpace(sec.MetaDSN); p != "" {
		return p
	}
	if dir := strings.TrimSpace(os.Getenv("CONGEE_DATA_DIR")); dir != "" {
		dbType := strings.TrimSpace(sec.Type)
		if dbType == "" || dbType == "sqlite" {
			return filepath.Join(filepath.Clean(dir), "congee-meta.db")
		}
	}
	dbType := strings.TrimSpace(sec.Type)
	if dbType == "" {
		dbType = "sqlite"
	}
	if dbType == "postgres" {
		return "./congee-meta.db"
	}
	eventsPath, err := sqliteEventsFilePath(sec.DSN)
	if err != nil || eventsPath == "" {
		return "./congee-meta.db"
	}
	return filepath.Join(filepath.Dir(eventsPath), "congee-meta.db")
}

func sqliteEventsFilePath(rawDSN string) (string, error) {
	dsn := strings.TrimSpace(rawDSN)
	if dsn == "" {
		return "", nil
	}
	if !strings.HasPrefix(dsn, "file:") {
		dsn = "file:" + dsn + "?cache=shared"
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", nil
	}
	if p := strings.TrimSpace(u.Path); p != "" && p != "/" {
		return filepath.Clean(p), nil
	}
	name := strings.TrimSpace(u.Opaque)
	if name == "" {
		return "", nil
	}
	name = strings.TrimPrefix(name, "./")
	if filepath.IsAbs(name) {
		return filepath.Clean(name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(cwd, name)), nil
}
