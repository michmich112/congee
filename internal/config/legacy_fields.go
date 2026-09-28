package config

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/rs/zerolog"
)

var (
	warnNIP11PubkeyOnce sync.Once
	warnNIP42AuthOnce   sync.Once
)

func warnLegacyNIP11Pubkey(data []byte) {
	if !nip11HasKey(data, "pubkey") {
		return
	}
	warnNIP11PubkeyOnce.Do(func() {
		log := legacyLogger()
		log.Warn().Msg("nip11.pubkey is ignored; the relay key is now nip-11 self")
	})
}

func applyLegacyNIP42RequireAuth(c *Config, data []byte) {
	if c == nil || !nip42HasKey(data, "send_challenge_on_connect") {
		return
	}
	warnNIP42AuthOnce.Do(func() {
		log := legacyLogger()
		log.Warn().Msg("nip42.send_challenge_on_connect is ignored; connection-wide auth is now an explicit nip42.require_auth choice")
	})
	if c.NIP42.RequireAuth == "" {
		c.NIP42.RequireAuth = NIP42RequireAuthProtectedKinds
	}
}

func nip11HasKey(data []byte, key string) bool {
	section, ok := rawSection(data, "nip11")
	if !ok {
		return false
	}
	_, ok = section[key]
	return ok
}

func nip42HasKey(data []byte, key string) bool {
	section, ok := rawSection(data, "nip42")
	if !ok {
		return false
	}
	_, ok = section[key]
	return ok
}

func rawSection(data []byte, name string) (map[string]json.RawMessage, bool) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false
	}
	raw, ok := root[name]
	if !ok {
		return nil, false
	}
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil {
		return nil, false
	}
	return section, true
}

func legacyLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).With().Timestamp().Logger()
}
