package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func TestPreflightMigrationTargetPostgresCurrent(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-preflight-pg")
	st, err := Open(ctx, dsn, "test-preflight-pg", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	out := PreflightMigrationTarget(ctx, dsn, zerolog.Nop())
	if out.Status != storage.MigrationPreflightCurrent {
		t.Fatalf("status=%q detail=%q", out.Status, out.Detail)
	}
	if out.ExpectedVersion != CurrentSchemaVersion() {
		t.Fatalf("expected_version=%d", out.ExpectedVersion)
	}
	if out.ReportedVersion == nil || *out.ReportedVersion != CurrentSchemaVersion() {
		t.Fatalf("reported_version=%v", out.ReportedVersion)
	}
}

func TestPreflightMigrationTargetPostgresEmptyDSN(t *testing.T) {
	ctx := context.Background()
	out := PreflightMigrationTarget(ctx, "  ", zerolog.Nop())
	if out.Status != storage.MigrationPreflightUnreadable || !strings.Contains(out.Detail, "empty") {
		t.Fatalf("got status=%q detail=%q", out.Status, out.Detail)
	}
}
