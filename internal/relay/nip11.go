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
	MaxFilters       int  `json:"max_filters"`
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
		Banner:        nip11ImageURL(r, h.Cfg.NIP11, config.NIP11AssetBanner),
		Icon:          nip11ImageURL(r, h.Cfg.NIP11, config.NIP11AssetIcon),
		PubKey:        h.Cfg.NIP11.AdminPubKey,
		Self:          self,
		Contact:       strings.TrimSpace(h.Cfg.NIP11.Contact),
		SupportedNIPs: supported,
		Software:      h.Cfg.NIP11.Software,
		Version:       version.Version,
		Limitation: nip11Limitation{
			MaxMessageLength: h.Cfg.WebSocket.MaxMessageBytes,
			MaxSubscriptions: h.Cfg.ConnectionLimits.MaxSubscriptionsPerConnection,
			MaxSubIDLength:   h.Cfg.MaxSubscriptionIDLength,
			MaxFilters:       h.Cfg.ConnectionLimits.MaxFiltersPerReq,
			DefaultLimit:     defaultLimit,
			AuthRequired:     config.NIP11AuthRequired(h.Cfg),
		},
	}
	w.Header().Set("Content-Type", "application/nostr+json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(doc)
}

func nip11ImageURL(r *http.Request, section config.NIP11Section, asset string) string {
	if section.ImageSource(asset) == config.NIP11ImageSourceURL {
		if asset == config.NIP11AssetBanner {
			return strings.TrimSpace(section.Banner)
		}
		return strings.TrimSpace(section.Icon)
	}
	path := "/assets/icon"
	if asset == config.NIP11AssetBanner {
		path = "/assets/banner"
	}
	return nip11AbsoluteURL(r, path)
}

func nip11AbsoluteURL(r *http.Request, path string) string {
	return requestScheme(r) + "://" + r.Host + path
}

func requestScheme(r *http.Request) string {
	if proto := forwardedProto(r); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	return "http"
}

func forwardedProto(r *http.Request) string {
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		return ""
	}
	if i := strings.Index(proto, ","); i >= 0 {
		proto = proto[:i]
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto != "http" && proto != "https" {
		return ""
	}
	return proto
}

// AcceptsNostrJSON reports whether the request asks for NIP-11 JSON.
func AcceptsNostrJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/nostr+json")
}
