package storage

import "github.com/michmich112/congee/internal/nostr"

// FilterSQLLimit returns the positive filter limit to use in SQL, or nil when no LIMIT clause
// should be applied. When applyLimits is false, nil is always returned.
//
// A nil or non-positive filter limit means unlimited rows at the SQL layer (callers omit LIMIT).
// This avoids sentinel integers colliding with legitimate large limits (e.g. math.MaxInt32).
func FilterSQLLimit(f *nostr.Filter, applyLimits bool) *int {
	if !applyLimits {
		return nil
	}
	if f != nil && f.Limit != nil && *f.Limit > 0 {
		v := *f.Limit
		return &v
	}
	return nil
}
