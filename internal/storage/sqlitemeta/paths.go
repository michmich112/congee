package sqlitemeta

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func mainFilePath(rawDSN string) (string, error) {
	s := normalizeDSN(rawDSN)
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("sqlitemeta: parse dsn: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("sqlitemeta: expected file: scheme in dsn")
	}
	if p := strings.TrimSpace(u.Path); p != "" && p != "/" {
		return filepath.Clean(p), nil
	}
	name := strings.TrimSpace(u.Opaque)
	if name == "" {
		return "", fmt.Errorf("sqlitemeta: empty path in file dsn")
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

func onDiskBytes(mainPath string) int64 {
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
