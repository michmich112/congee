package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

var hexEventIDRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

type storedEventListItem struct {
	ID        string `json:"id"`
	PubKey    string `json:"pubkey"`
	CreatedAt int64  `json:"created_at"`
	Kind      int    `json:"kind"`
}

func handleGetEvent(st storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.ToLower(r.PathValue("id"))
		if id == "" || !hexEventIDRe.MatchString(id) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid event id"}`))
			return
		}
		lim := 1
		evs, err := st.QueryEvents(r.Context(), []nostr.Filter{{IDs: []string{id}, Limit: &lim}})
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"query failed"}`))
			return
		}
		if len(evs) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"event": evs[0]})
	}
}

// handleListEvents serves GET /events — stored relay events by kind (event store, not audit_log).
func handleListEvents(st storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		kinds := parseAuditKindsQuery(r)
		if len(kinds) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"kind is required"}`))
			return
		}
		limit := parseAuditLimit(r.URL.Query().Get("limit"))
		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		f := nostr.Filter{Kinds: kinds}
		if pk := strings.TrimSpace(r.URL.Query().Get("pubkey")); pk != "" {
			f.Authors = []string{strings.ToLower(pk)}
		}
		if v := r.URL.Query().Get("since"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				f.Since = &n
			}
		}
		if v := r.URL.Query().Get("until"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				f.Until = &n
			}
		}
		ctx := r.Context()
		total, err := st.CountEvents(ctx, []nostr.Filter{f})
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"count failed"}`))
			return
		}
		fetch := offset + limit
		if fetch < 1 {
			fetch = limit
		}
		f.Limit = &fetch
		evs, err := st.QueryEvents(ctx, []nostr.Filter{f})
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"query failed"}`))
			return
		}
		if offset > len(evs) {
			evs = evs[:0]
		} else {
			evs = evs[offset:]
			if len(evs) > limit {
				evs = evs[:limit]
			}
		}
		out := make([]storedEventListItem, 0, len(evs))
		for _, ev := range evs {
			if ev == nil {
				continue
			}
			out = append(out, storedEventListItem{
				ID:        ev.ID,
				PubKey:    ev.PubKey,
				CreatedAt: ev.CreatedAt,
				Kind:      ev.Kind,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"events": out, "total": total})
	}
}
