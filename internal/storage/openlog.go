package storage

import (
	"context"

	"github.com/rs/zerolog"
)

type openLoggerKey struct{}

// WithOpenLogger attaches a logger for optional debug traces inside sqlite.Open and postgres.Open
// (ping, migrations, notifier startup). When absent, those packages stay silent.
func WithOpenLogger(ctx context.Context, log zerolog.Logger) context.Context {
	return context.WithValue(ctx, openLoggerKey{}, log)
}

// OpenLogger returns the logger passed via WithOpenLogger, if any.
func OpenLogger(ctx context.Context) (zerolog.Logger, bool) {
	v := ctx.Value(openLoggerKey{})
	if v == nil {
		return zerolog.Logger{}, false
	}
	l, ok := v.(zerolog.Logger)
	return l, ok
}
