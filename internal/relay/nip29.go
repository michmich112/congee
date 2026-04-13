package relay

import (
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// RegisterNIP29 registers NIP-29 relay groups (validators and hooks added in a follow-up commit).
func RegisterNIP29(_ *Server, _ storage.Store, _ zerolog.Logger) {}
