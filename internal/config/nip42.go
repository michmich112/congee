package config

import (
	"fmt"
	"net/url"
	"strings"
)

// ApplyNIP42RelayURLs validates the canonical relay URL and every alias with
// NormalizeNIP42RelayURL, drops aliases that repeat the canonical URL or an earlier
// alias, and stores the normalized forms. requireCanonical is true when NIP-42 is enabled.
func ApplyNIP42RelayURLs(relayURL *string, aliases *[]string, requireCanonical bool) error {
	if relayURL == nil || aliases == nil {
		return fmt.Errorf("config: nip42 relay URL fields are missing")
	}
	rawURL := strings.TrimSpace(*relayURL)
	if rawURL == "" {
		if requireCanonical {
			return fmt.Errorf("config: nip42.relay_url is required when NIP 42 is enabled")
		}
		*relayURL = ""
	} else {
		normalized, err := NormalizeNIP42RelayURL(rawURL)
		if err != nil {
			return fmt.Errorf("config: nip42.relay_url: %w", err)
		}
		*relayURL = normalized
	}

	seen := map[string]struct{}{}
	if *relayURL != "" {
		seen[*relayURL] = struct{}{}
	}
	if len(*aliases) == 0 {
		return nil
	}
	out := make([]string, 0, len(*aliases))
	for i, alias := range *aliases {
		normalized, err := NormalizeNIP42RelayURL(alias)
		if err != nil {
			return fmt.Errorf("config: nip42.relay_aliases[%d]: %w", i, err)
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	*aliases = out
	return nil
}

// NormalizeNIP42RelayURL returns a canonical relay URL string for NIP-42 relay tag comparison.
// It is the single check for nip42.relay_url and each nip42.relay_aliases entry.
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
