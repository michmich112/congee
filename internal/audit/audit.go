package audit

import (
	"context"
	"time"

	"github.com/michmich112/congee/internal/storage"
)

// Log persists one audit row (full pubkey; put conn_id in detail if needed).
func Log(ctx context.Context, st storage.Store, action, detail, pubkey string) error {
	return st.SaveAuditEntry(ctx, storage.AuditEntry{
		CreatedAt: time.Now().Unix(),
		Action:    action,
		Detail:    detail,
		Pubkey:    pubkey,
	})
}
