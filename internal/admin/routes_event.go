package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

var hexEventIDRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
