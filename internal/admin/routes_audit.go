package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/michmich112/congee/internal/storage"
)

// maxAuditQueryLimit caps GET /api/audit ?limit= for bounded response size (admin-only).
const maxAuditQueryLimit = 5000

// parseAuditLimit returns the effective page size from ?limit=; default 50, minimum 1, capped at maxAuditQueryLimit.
// Values above the cap clamp instead of falling back to default so a typo like 10000 still returns a large page.
func parseAuditLimit(raw string) int {
	const defaultLimit = 50
	if raw == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultLimit
	}
	if n > maxAuditQueryLimit {
		return maxAuditQueryLimit
	}
	return n
}

func handleAudit(st storage.Store) http.HandlerFunc {
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

		rows, err := st.QueryAuditLog(r.Context(), q)
		if err != nil {
			http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": rows})
	}
}
