package sqlitemeta

import (
	"context"
	"fmt"
)

// AnalyzeStatsTables refreshes sqlite_stat1 for tables used by the admin storage snapshot.
func (s *Store) AnalyzeStatsTables(ctx context.Context) error {
	if _, err := s.db().ExecContext(ctx, `ANALYZE audit_log`); err != nil {
		return fmt.Errorf("sqlitemeta: ANALYZE audit_log: %w", err)
	}
	return nil
}
