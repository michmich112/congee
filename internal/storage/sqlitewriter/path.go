package sqlitewriter

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ResolveMainFilePath resolves the on-disk database file path from a file: DSN or bare path.
func ResolveMainFilePath(rawDSN string) (string, error) {
	s := NormalizeDSN(rawDSN)
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("sqlitewriter: parse dsn: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("sqlitewriter: expected file: scheme in dsn")
	}
	if p := strings.TrimSpace(u.Path); p != "" && p != "/" {
		return filepath.Clean(p), nil
	}
	name := strings.TrimSpace(u.Opaque)
	if name == "" {
		return "", fmt.Errorf("sqlitewriter: empty path in file dsn")
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
