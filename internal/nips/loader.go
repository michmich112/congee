package nips

import (
	"fmt"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// LoadEnabled registers pipeline components for each enabled NIP in config.
func LoadEnabled(cfg *config.Config, s *relay.Server, store storage.Store, log zerolog.Logger) error {
	relay.RegisterNIP01(s, store, log)
	seen := map[int]struct{}{1: {}}
	for _, n := range cfg.NIPs.Enabled {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		switch n {
		case 50:
			relay.RegisterNIP50(s, store, log)
		default:
			return fmt.Errorf("nips: NIP %d is not implemented in the loader", n)
		}
	}
	return nil
}

// IsImplemented reports whether the relay loader can register this NIP today.
func IsImplemented(n int) bool {
	switch n {
	case 1, 50:
		return true
	default:
		return false
	}
}
