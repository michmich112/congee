package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/michmich112/congee/internal/storage"
)

// parseAuditLimit returns the effective page size from ?limit= (default 50 when missing or invalid).
// There is no maximum: pagination uses offset; callers should choose a sensible page size.
func parseAuditLimit(raw string) int {
	const defaultLimit = 50
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultLimit
	}
	return n
}

// HandleAudit serves GET /audit (mounted under /api/ after StripPrefix, wrapped by RequireAdminAuth).
func HandleAudit(st storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := storage.AuditQuery{Limit: parseAuditLimit(r.URL.Query().Get("limit"))}
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				q.Offset = n
			}
		}
		if v := r.URL.Query().Get("since"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				q.Since = n
			}
		}
		if v := r.URL.Query().Get("until"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				q.Until = n
			}
		}
		q.Action = r.URL.Query().Get("action")
		q.Pubkey = r.URL.Query().Get("pubkey")

		ctx := r.Context()
		total, err := st.CountAuditLog(ctx, q)
		if err != nil {
			http.Error(w, `{"error":"count failed"}`, http.StatusInternalServerError)
			return
		}
		rows, err := st.QueryAuditLog(ctx, q)
		if err != nil {
			http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": rows, "total": total})
	}
}
