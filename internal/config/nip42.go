package config

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeNIP42RelayURL returns a canonical relay URL string for NIP-42 relay tag comparison.
// Scheme must be ws or wss; host is lowercased; path defaults to "/" and trailing slashes (except a lone "/") are trimmed.
func NormalizeNIP42RelayURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return "", fmt.Errorf("scheme must be ws or wss")
	}
	host := strings.ToLower(u.Host)
	path := u.Path
	if path == "" {
		path = "/"
	}
	for len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return scheme + "://" + host + path, nil
}
