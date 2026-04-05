package relay

import (
	"net/http"

	"github.com/michmich112/congee/internal/storage"
)

// HealthHandler answers GET /health when the store is reachable.
type HealthHandler struct {
	Store storage.Store
}

// ServeHTTP returns 200 with body "ok" if CountEvents succeeds.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	if _, err := h.Store.CountEvents(ctx, nil); err != nil {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
