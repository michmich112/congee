package admin

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireAdminAuth enforces ADMIN_PASSWORD via Authorization: Bearer <token> or X-Admin-Token.
// Documented here: both headers compare to the same env value; use HTTPS in production.
func RequireAdminAuth(password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if password == "" {
			http.Error(w, `{"error":"admin password not configured"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		if token == "" {
			token = strings.TrimSpace(r.Header.Get("X-Admin-Token"))
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(password)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
