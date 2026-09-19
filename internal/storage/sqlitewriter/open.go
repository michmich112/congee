package sqlitewriter

import "strings"

// NormalizeDSN returns a file: DSN with shared cache for a local libSQL database.
func NormalizeDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "file:congee.db?cache=shared"
	}
	if strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	return "file:" + dsn + "?cache=shared"
}
