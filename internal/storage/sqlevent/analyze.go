package sqlevent

import (
	"context"
	"fmt"
)

// AnalyzeStatsTables refreshes sqlite_stat1 for tables used by the admin storage snapshot.
func (s *Store) AnalyzeStatsTables(ctx context.Context) error {
	for _, q := range []string{`ANALYZE events`, `ANALYZE event_tags`} {
		if _, err := s.db().ExecContext(ctx, q); err != nil {
			return fmt.Errorf("%s: %s: %w", s.engine, q, err)
		}
	}
	return nil
}
