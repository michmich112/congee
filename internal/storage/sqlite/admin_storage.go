package sqlite

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/michmich112/congee/internal/storage"
)

func sqliteMainFilePath(rawDSN string) (string, error) {
	s := normalizeDSN(rawDSN)
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("sqlite: parse dsn: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("sqlite: expected file: scheme in dsn")
	}
	if p := strings.TrimSpace(u.Path); p != "" && p != "/" {
		return filepath.Clean(p), nil
	}
	name := strings.TrimSpace(u.Opaque)
	if name == "" {
		return "", fmt.Errorf("sqlite: empty path in file dsn")
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

func sqliteOnDiskBytes(mainPath string) int64 {
	var sum int64
	for _, p := range []string{mainPath, mainPath + "-wal", mainPath + "-shm"} {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			sum += st.Size()
		}
	}
	return sum
}

// AdminStorageSnapshot implements storage.Store.
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	out.Bytes = sqliteOnDiskBytes(s.dbPath)
	row := s.db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM events) AS ev,
  (SELECT COUNT(*) FROM event_tags) AS tg
`)
	if err := row.Scan(&out.Events, &out.Tags); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("sqlite: admin snapshot counts: %w", err)
	}
	return out, nil
}
