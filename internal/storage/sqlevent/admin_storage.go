package sqlevent

import (
	"context"
	"fmt"
	"os"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/approxrows"
)

func sqliteOnDiskBytes(mainPath string) int64 {
	var sum int64
	for _, p := range []string{mainPath, mainPath + "-wal", mainPath + "-shm"} {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			sum += st.Size()
		}
	}
	return sum
}

// AdminStorageSnapshot implements storage.Store.
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	out.Bytes = sqliteOnDiskBytes(s.dbPath)
	var err error
	out.Events, err = approxrows.SQLiteTable(ctx, s.db(), "events")
	if err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("%s: admin snapshot events count: %w", s.engine, err)
	}
	out.Tags, err = approxrows.SQLiteTable(ctx, s.db(), "event_tags")
	if err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("%s: admin snapshot tags count: %w", s.engine, err)
	}
	return out, nil
}
