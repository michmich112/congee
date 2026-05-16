package relay

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

// RegisterNIP50 replaces the REQ handler with one that enables NIP-50 search filters.
// Call after RegisterNIP01. Requires nip 50 in config nips.enabled (loader enforces order).
func RegisterNIP50(s *Server, _ storage.Store) {
	s.RegisterMessageHandler("REQ", func(ctx context.Context, c *Conn, msg any) error {
		return handleREQ(ctx, s, c, msg.(*nostr.ReqMessage), true)
	})
}
