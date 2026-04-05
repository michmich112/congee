package storage

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// MigrationCounts are row totals used for progress and verification when copying between stores.
type MigrationCounts struct {
	Events    int64
	Tags      int64
	Audit     int64
	Changelog int64
}

// MigrationProgress is reported during a copy (percent 0–100, human message).
type MigrationProgress struct {
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
}

// MigrationSource is implemented by stores that support bulk export for migration tooling.
type MigrationSource interface {
	Store
	MigrationRowCounts(ctx context.Context) (MigrationCounts, error)
	ScanEventsForMigration(ctx context.Context, fn func(ev *nostr.Event) error) error
	ScanAuditForMigration(ctx context.Context, fn func(AuditEntry) error) error
	ScanChangelogForMigration(ctx context.Context, fn func(ConfigChange) error) error
}
