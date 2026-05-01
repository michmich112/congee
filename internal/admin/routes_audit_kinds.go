package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/michmich112/congee/internal/storage"
)

// parseAuditKindsQuery collects kind= from the query string. Repeated keys, comma-separated
// values, and duplicates are accepted; invalid tokens are skipped.
func parseAuditKindsQuery(r *http.Request) []int {
	var raw []string
	for _, v := range r.URL.Query()["kind"] {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			raw = append(raw, part)
		}
	}
	var out []int
	for _, s := range raw {
		if k, err := strconv.Atoi(s); err == nil && k >= 0 {
			out = append(out, k)
		}
	}
	return storage.DedupeSortNonNegInts(out)
}

// HandleAuditKinds serves GET /audit/kinds (under /api/ after StripPrefix). JSON body: {"kinds":[int,...]}.
// Optional ?scan_limit= caps how many newest audit rows are scanned (default from storage, max storage.MaxAuditKindsScanLimit).
func HandleAuditKinds(st storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		scanLimit := storage.DefaultAuditKindsScanLimit
		if v := r.URL.Query().Get("scan_limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				scanLimit = n
			}
		}
		ctx := r.Context()
		kinds, err := st.ListDistinctAuditKinds(ctx, scanLimit)
		if err != nil {
			http.Error(w, `{"error":"kinds query failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"kinds": kinds})
	}
}
