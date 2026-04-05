package admin

import (
	"os"
	"strings"
)

// Enabled reports whether ENABLE_ADMIN_UI is truthy (case-insensitive for common tokens).
func Enabled() bool {
	v := strings.TrimSpace(os.Getenv("ENABLE_ADMIN_UI"))
	if v == "" {
		return false
	}
	if strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") || strings.EqualFold(v, "on") {
		return true
	}
	switch v {
	case "1":
		return true
	default:
		return false
	}
}

// AdminPassword returns ADMIN_PASSWORD from the environment (may be empty).
func AdminPassword() string {
	return os.Getenv("ADMIN_PASSWORD")
}

// DevEnvironment returns lowercased CONGEE_ENV (may be empty).
func DevEnvironment() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("CONGEE_ENV")))
}

func isDevEnv() bool {
	switch DevEnvironment() {
	case "dev", "development", "local":
		return true
	default:
		return false
	}
}
