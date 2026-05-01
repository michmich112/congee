package storage

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// MigrationCounts are row totals used for progress and verification when copying between stores.
type MigrationCounts struct {
	Events    int64 `json:"events"`
	Tags      int64 `json:"tags"`
	Audit     int64 `json:"audit"`
	Changelog int64 `json:"changelog"`
}

// MigrationProgress is reported during a copy (percent 0–100, human message).
type MigrationProgress struct {
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
}

// MigrationSummary is returned after a successful Migrate run for operator feedback.
type MigrationSummary struct {
	Source           MigrationCounts `json:"source"`
	DestinationFinal MigrationCounts `json:"destination_final"`
	EventsInserted   int64           `json:"events_inserted"`
	EventsSkipped    int64           `json:"events_skipped"`
	TagsAdded        int64           `json:"tags_added"`
	AuditInserted    int64           `json:"audit_inserted"`
	AuditSkipped     int64           `json:"audit_skipped"`
	ChangelogCopied  int64           `json:"changelog_copied"`
}

// MigrationSource is implemented by stores that support bulk export for migration tooling.
type MigrationSource interface {
	Store
	MigrationRowCounts(ctx context.Context) (MigrationCounts, error)
	ScanEventsForMigration(ctx context.Context, fn func(ev *nostr.Event) error) error
	ScanAuditForMigration(ctx context.Context, fn func(AuditEntry) error) error
	ScanChangelogForMigration(ctx context.Context, fn func(ConfigChange) error) error
}
