package nips

import (
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips/registry"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// LoadEnabled registers pipeline components for each enabled NIP in config.
func LoadEnabled(cfg *config.Config, s *relay.Server, store storage.Store, log zerolog.Logger) error {
	return registry.Load(cfg, s, store, log)
}

// IsImplemented reports whether the relay loader can register this NIP today.
func IsImplemented(n int) bool {
	switch n {
	case 1, 2, 11, 29, 42, 50:
		return true
	default:
		return false
	}
}
