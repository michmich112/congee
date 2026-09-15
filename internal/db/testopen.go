package db

import (
	"context"
	"errors"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

// OpenForTest opens a Turso/libSQL events database plus meta sidecar for tests.
func OpenForTest(ctx context.Context, eventsDSN string, log zerolog.Logger) (*Handle, error) {
	if !turso.HasDriver() {
		return nil, errors.New("turso: libsql driver not available (build with CGO_ENABLED=1)")
	}
	sec := config.DatabaseSection{Type: "turso", DSN: eventsDSN}
	return openTurso(ctx, sec, log)
}

// OpenTestStore is a convenience wrapper returning the composed Store and close func.
func OpenTestStore(ctx context.Context, eventsDSN string, log zerolog.Logger) (storage.Store, func() error, error) {
	h, err := OpenForTest(ctx, eventsDSN, log)
	if err != nil {
		return nil, nil, err
	}
	return h.Store, h.Close, nil
}
