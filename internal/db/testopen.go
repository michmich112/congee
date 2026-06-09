package db

import (
	"context"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// OpenForTest opens a SQLite events database plus meta sidecar for tests.
func OpenForTest(ctx context.Context, eventsDSN string, log zerolog.Logger) (*Handle, error) {
	sec := config.DatabaseSection{Type: "sqlite", DSN: eventsDSN}
	return openSQLite(ctx, sec, log)
}

// OpenTestStore is a convenience wrapper returning the composed Store and close func.
func OpenTestStore(ctx context.Context, eventsDSN string, log zerolog.Logger) (storage.Store, func() error, error) {
	h, err := OpenForTest(ctx, eventsDSN, log)
	if err != nil {
		return nil, nil, err
	}
	return h.Store, h.Close, nil
}
