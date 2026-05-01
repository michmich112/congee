package storage

import "context"

type bulkMigrationCtxKey struct{}

// WithBulkMigration marks ctx so the PostgreSQL store skips pg_notify after each
// SaveEvent during admin data migration (one round-trip per event otherwise).
// Other backends ignore this flag.
func WithBulkMigration(ctx context.Context) context.Context {
	return context.WithValue(ctx, bulkMigrationCtxKey{}, true)
}

// IsBulkMigration reports whether WithBulkMigration was applied.
func IsBulkMigration(ctx context.Context) bool {
	v, _ := ctx.Value(bulkMigrationCtxKey{}).(bool)
	return v
}
