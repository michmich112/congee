package db

import (
	"context"

	"github.com/michmich112/congee/internal/config"
	"github.com/rs/zerolog"
)

// OpenForTest opens a SQLite events database plus meta sidecar for tests.
func OpenForTest(ctx context.Context, eventsDSN string, log zerolog.Logger) (*Handle, error) {
	sec := config.DatabaseSection{Type: "sqlite", DSN: eventsDSN}
	return openSQLite(ctx, sec, log)
}
