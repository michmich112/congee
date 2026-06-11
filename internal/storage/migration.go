package storage

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// MigrationCounts are row totals used for progress and verification when copying between stores.
type MigrationCounts struct {
	Events int64 `json:"events"`
	Tags   int64 `json:"tags"`
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
}

// MigrationSource is implemented by event stores that support bulk export for migration tooling.
type MigrationSource interface {
	EventStore
	MigrationRowCounts(ctx context.Context) (MigrationCounts, error)
	ScanEventsForMigration(ctx context.Context, fn func(ev *nostr.Event) error) error
}
