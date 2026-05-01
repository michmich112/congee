package storage

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/nostr"
)

// Migrate copies events (with tags), audit_log, and config_changelog from src to dst.
// dst must be empty or compatible (duplicate event ids will fail on SaveEvent).
// progress is optional; it may be called from the caller's goroutine frequently during the run.
// debug is optional; when non-nil it receives short milestone strings (not per-row).
func Migrate(ctx context.Context, src, dst MigrationSource, progress func(MigrationProgress), debug func(string)) error {
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
		return fmt.Errorf("migration: source counts: %w", err)
	}
	debug(fmt.Sprintf("source_row_counts events=%d tags=%d audit=%d changelog=%d",
		srcCounts.Events, srcCounts.Tags, srcCounts.Audit, srcCounts.Changelog))

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
	var evCopied int64
	err = src.ScanEventsForMigration(ctx, func(ev *nostr.Event) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := dst.SaveEvent(ctx, ev); err != nil {
			return fmt.Errorf("migration: save event %s: %w", ev.ID, err)
		}
		evCopied++
		done++
		if srcCounts.Events > 0 {
			report(fmt.Sprintf("events %d / %d", evCopied, srcCounts.Events))
		}
		return nil
	})
	if err != nil {
		return err
	}
	debug(fmt.Sprintf("events_copy_done copied=%d", evCopied))

	progress(MigrationProgress{Percent: 40, Message: fmt.Sprintf("copying audit_log (0 / %d)", srcCounts.Audit)})
	var auditCopied int64
	err = src.ScanAuditForMigration(ctx, func(e AuditEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := dst.SaveAuditEntry(ctx, e); err != nil {
			return fmt.Errorf("migration: save audit: %w", err)
		}
		auditCopied++
		done++
		if srcCounts.Audit > 0 {
			report(fmt.Sprintf("audit %d / %d", auditCopied, srcCounts.Audit))
		}
		return nil
	})
	if err != nil {
		return err
	}
	debug(fmt.Sprintf("audit_copy_done copied=%d", auditCopied))

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
		return err
	}
	debug(fmt.Sprintf("changelog_copy_done copied=%d", chCopied))

	progress(MigrationProgress{Percent: 92, Message: "verifying row counts"})
	dstCounts, err := dst.MigrationRowCounts(ctx)
	if err != nil {
		return fmt.Errorf("migration: destination counts: %w", err)
	}
	if dstCounts.Events != srcCounts.Events || dstCounts.Tags != srcCounts.Tags ||
		dstCounts.Audit != srcCounts.Audit || dstCounts.Changelog != srcCounts.Changelog {
		return fmt.Errorf("migration: count mismatch src=%+v dst=%+v", srcCounts, dstCounts)
	}
	debug(fmt.Sprintf("verify_ok dst events=%d tags=%d audit=%d changelog=%d",
		dstCounts.Events, dstCounts.Tags, dstCounts.Audit, dstCounts.Changelog))

	progress(MigrationProgress{Percent: 100, Message: "done"})
	return nil
}
