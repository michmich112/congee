package storage

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/nostr"
)

// Migrate copies events (with tags), audit_log, and config_changelog from src to dst.
// Events and audit rows already present on dst (same event id, or identical audit row) are skipped.
// dst may be pre-populated; verification compares row deltas against dst's counts at start.
// progress is optional; it may be called from the caller's goroutine frequently during the run.
// debug is optional; when non-nil it receives short milestone strings (not per-row).
// On success, the returned summary describes rows copied and final destination totals.
func Migrate(ctx context.Context, src, dst MigrationSource, progress func(MigrationProgress), debug func(string)) (MigrationSummary, error) {
	if progress == nil {
		progress = func(MigrationProgress) {}
	}
	if debug == nil {
		debug = func(string) {}
	}

	ctx = WithBulkMigration(ctx)

	progress(MigrationProgress{Percent: 0, Message: "counting source rows"})
	srcCounts, err := src.MigrationRowCounts(ctx)
	if err != nil {
		return MigrationSummary{}, fmt.Errorf("migration: source counts: %w", err)
	}
	dstStart, err := dst.MigrationRowCounts(ctx)
	if err != nil {
		return MigrationSummary{}, fmt.Errorf("migration: destination baseline counts: %w", err)
	}
	debug(fmt.Sprintf("source_row_counts events=%d tags=%d audit=%d changelog=%d",
		srcCounts.Events, srcCounts.Tags, srcCounts.Audit, srcCounts.Changelog))
	debug(fmt.Sprintf("dst_start_counts events=%d tags=%d audit=%d changelog=%d",
		dstStart.Events, dstStart.Tags, dstStart.Audit, dstStart.Changelog))

	totalSteps := srcCounts.Events + srcCounts.Audit + srcCounts.Changelog
	if totalSteps == 0 {
		totalSteps = 1
	}
	var done int64

	report := func(msg string) {
		p := float64(done) / float64(totalSteps) * 100
		if p > 100 {
			p = 100
		}
		progress(MigrationProgress{Percent: p, Message: msg})
	}

	progress(MigrationProgress{Percent: 1, Message: fmt.Sprintf("copying events (0 / %d)", srcCounts.Events)})
	var evInserted, evSkipped int64
	var tagsAdded int64
	err = src.ScanEventsForMigration(ctx, func(ev *nostr.Event) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		exists, err := dst.HasEventID(ctx, ev.ID)
		if err != nil {
			return fmt.Errorf("migration: check event %s: %w", ev.ID, err)
		}
		if exists {
			evSkipped++
			done++
			if srcCounts.Events > 0 {
				report(fmt.Sprintf("events %d / %d (%d skipped)", evInserted+evSkipped, srcCounts.Events, evSkipped))
			}
			return nil
		}
		if err := dst.SaveEvent(ctx, ev); err != nil {
			return fmt.Errorf("migration: save event %s: %w", ev.ID, err)
		}
		evInserted++
		tagsAdded += int64(len(ev.Tags))
		done++
		if srcCounts.Events > 0 {
			report(fmt.Sprintf("events %d / %d (%d skipped)", evInserted+evSkipped, srcCounts.Events, evSkipped))
		}
		return nil
	})
	if err != nil {
		return MigrationSummary{}, err
	}
	debug(fmt.Sprintf("events_copy_done inserted=%d skipped=%d", evInserted, evSkipped))

	progress(MigrationProgress{Percent: 40, Message: fmt.Sprintf("copying audit_log (0 / %d)", srcCounts.Audit)})
	var auditInserted, auditSkipped int64
	err = src.ScanAuditForMigration(ctx, func(e AuditEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dup, err := dst.HasAuditDuplicate(ctx, e)
		if err != nil {
			return fmt.Errorf("migration: check audit: %w", err)
		}
		if dup {
			auditSkipped++
			done++
			if srcCounts.Audit > 0 {
				report(fmt.Sprintf("audit %d / %d (%d skipped)", auditInserted+auditSkipped, srcCounts.Audit, auditSkipped))
			}
			return nil
		}
		if err := dst.SaveAuditEntry(ctx, e); err != nil {
			return fmt.Errorf("migration: save audit: %w", err)
		}
		auditInserted++
		done++
		if srcCounts.Audit > 0 {
			report(fmt.Sprintf("audit %d / %d (%d skipped)", auditInserted+auditSkipped, srcCounts.Audit, auditSkipped))
		}
		return nil
	})
	if err != nil {
		return MigrationSummary{}, err
	}
	debug(fmt.Sprintf("audit_copy_done inserted=%d skipped=%d", auditInserted, auditSkipped))

	progress(MigrationProgress{Percent: 70, Message: fmt.Sprintf("copying config_changelog (0 / %d)", srcCounts.Changelog)})
	var chCopied int64
	err = src.ScanChangelogForMigration(ctx, func(c ConfigChange) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := dst.SaveConfigChange(ctx, c); err != nil {
			return fmt.Errorf("migration: save changelog: %w", err)
		}
		chCopied++
		done++
		if srcCounts.Changelog > 0 {
			report(fmt.Sprintf("changelog %d / %d", chCopied, srcCounts.Changelog))
		}
		return nil
	})
	if err != nil {
		return MigrationSummary{}, err
	}
	debug(fmt.Sprintf("changelog_copy_done copied=%d", chCopied))

	progress(MigrationProgress{Percent: 92, Message: "verifying row counts"})
	dstCounts, err := dst.MigrationRowCounts(ctx)
	if err != nil {
		return MigrationSummary{}, fmt.Errorf("migration: destination counts: %w", err)
	}
	if dstCounts.Audit != dstStart.Audit+auditInserted {
		return MigrationSummary{}, fmt.Errorf("migration: audit count mismatch start=%d inserted=%d dst=%d",
			dstStart.Audit, auditInserted, dstCounts.Audit)
	}
	if dstCounts.Changelog != dstStart.Changelog+chCopied {
		return MigrationSummary{}, fmt.Errorf("migration: changelog count mismatch start=%d copied=%d dst=%d",
			dstStart.Changelog, chCopied, dstCounts.Changelog)
	}
	debug(fmt.Sprintf("verify_ok dst events=%d tags=%d audit=%d changelog=%d (tags_added=%d from inserted events)",
		dstCounts.Events, dstCounts.Tags, dstCounts.Audit, dstCounts.Changelog, tagsAdded))

	progress(MigrationProgress{Percent: 100, Message: "done"})
	return MigrationSummary{
		Source:           srcCounts,
		DestinationFinal: dstCounts,
		EventsInserted:   evInserted,
		EventsSkipped:    evSkipped,
		TagsAdded:        tagsAdded,
		AuditInserted:    auditInserted,
		AuditSkipped:     auditSkipped,
		ChangelogCopied:  chCopied,
	}, nil
}
