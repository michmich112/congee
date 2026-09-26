package relay

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/version"
)

// NIP11Handler serves relay information when the client requests application/nostr+json.
type NIP11Handler struct {
	Cfg     *config.Config
	RelayID *relayidentity.Identity
}

type nip11Doc struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Banner        string          `json:"banner,omitempty"`
	Icon          string          `json:"icon,omitempty"`
	PubKey        string          `json:"pubkey,omitempty"`
	Self          string          `json:"self,omitempty"`
	Contact       string          `json:"contact,omitempty"`
	SupportedNIPs []int           `json:"supported_nips"`
	Software      string          `json:"software"`
	Version       string          `json:"version"`
	Limitation    nip11Limitation `json:"limitation"`
}

// Only advertise limits enforced by the relay. In particular, Congee does not
// clamp an explicit filter limit or cap the number of tags in an event.
type nip11Limitation struct {
	MaxMessageLength int  `json:"max_message_length"`
	MaxSubscriptions int  `json:"max_subscriptions"`
	MaxSubIDLength   int  `json:"max_subid_length"`
	DefaultLimit     *int `json:"default_limit,omitempty"`
	AuthRequired     bool `json:"auth_required"`
}

// writeNIP11CORSResponse sets CORS headers for browser NIP-11 fetches.
// Access-Control-Allow-Private-Network is required when the relay is reached via Tailscale,
// RFC1918, etc., and the page is on a public origin (Chrome Private Network Access).
func writeNIP11CORSResponse(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
}

// writeNIP11CORSPreflightHeaders sets CORS headers for OPTIONS preflight on GET / (NIP-11).
// When the browser sends Access-Control-Request-Headers, that value must be reflected in
// Access-Control-Allow-Headers or the preflight fails (some clients list more than Accept).
func writeNIP11CORSPreflightHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	if reqHdr := r.Header.Get("Access-Control-Request-Headers"); reqHdr != "" {
		w.Header().Set("Access-Control-Allow-Headers", reqHdr)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Accept")
	}
	// PNA preflight: browser sends Access-Control-Request-Private-Network: true
	if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
	}
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// ServeHTTP writes JSON metadata; callers should only invoke for GET / with matching Accept.
func (h *NIP11Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Cfg.NIP11.CORSAllowAnyOrigin {
		writeNIP11CORSResponse(w)
	}
	supported := slices.Clone(h.Cfg.NIPs.Enabled)
	slices.Sort(supported)
	var self string
	if h.RelayID != nil {
		self = h.RelayID.PubKeyHex()
	}
	var defaultLimit *int
	if limit := config.EffectiveREQDefaultQueryLimit(h.Cfg.ConnectionLimits.DefaultQueryLimit); limit > 0 {
		defaultLimit = &limit
	}

	doc := nip11Doc{
		Name:          h.Cfg.NIP11.Name,
		Description:   h.Cfg.NIP11.Description,
		Banner:        h.Cfg.NIP11.Banner,
		Icon:          h.Cfg.NIP11.Icon,
		PubKey:        h.Cfg.NIP11.AdminPubKey,
		Self:          self,
		Contact:       h.Cfg.NIP11.Contact,
		SupportedNIPs: supported,
		Software:      h.Cfg.NIP11.Software,
		Version:       version.Version,
		Limitation: nip11Limitation{
			MaxMessageLength: h.Cfg.WebSocket.MaxMessageBytes,
			MaxSubscriptions: h.Cfg.ConnectionLimits.MaxSubscriptionsPerConnection,
			MaxSubIDLength:   h.Cfg.MaxSubscriptionIDLength,
			DefaultLimit:     defaultLimit,
			AuthRequired:     false,
		},
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
