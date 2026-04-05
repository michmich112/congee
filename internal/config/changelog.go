package config

import (
	"context"
	"time"

	"github.com/michmich112/congee/internal/storage"
)

// SaveConfigChange records a config changelog row (CreatedAt is Unix seconds).
func SaveConfigChange(ctx context.Context, st storage.Store, summary, jsonDiff string) error {
	return st.SaveConfigChange(ctx, storage.ConfigChange{
		CreatedAt: time.Now().Unix(),
		Summary:   summary,
		JSONDiff:  jsonDiff,
	})
}
