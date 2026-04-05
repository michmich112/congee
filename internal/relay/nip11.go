package relay

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/version"
)

// NIP11Handler serves relay information when the client requests application/nostr+json.
type NIP11Handler struct {
	Cfg *config.Config
}

type nip11Doc struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	PubKey        string   `json:"pubkey"`
	Contact       string   `json:"contact"`
	SupportedNIPs []int    `json:"supported_nips"`
	Software      string   `json:"software"`
	Version       string   `json:"version"`
}

// ServeHTTP writes JSON metadata; callers should only invoke for GET / with matching Accept.
func (h *NIP11Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	supported := slices.Clone(h.Cfg.NIPs.Enabled)
	slices.Sort(supported)

	doc := nip11Doc{
		Name:          h.Cfg.NIP11.Name,
		Description:   h.Cfg.NIP11.Description,
		PubKey:        h.Cfg.NIP11.PubKey,
		Contact:       h.Cfg.NIP11.Contact,
		SupportedNIPs: supported,
		Software:      h.Cfg.NIP11.Software,
		Version:       version.Version,
	}
	w.Header().Set("Content-Type", "application/nostr+json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(doc)
}

// AcceptsNostrJSON reports whether the request asks for NIP-11 JSON.
func AcceptsNostrJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/nostr+json")
}
