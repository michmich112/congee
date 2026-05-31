package relay

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/version"
)

// NIP11Handler serves relay information when the client requests application/nostr+json.
type NIP11Handler struct {
	Cfg    *config.Config
	Server *Server
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

// writeNIP11CORSResponse sets CORS headers for browser NIP-11 fetches.
func writeNIP11CORSResponse(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
}

// writeNIP11CORSPreflightHeaders sets CORS headers for OPTIONS preflight on GET / (NIP-11).
func writeNIP11CORSPreflightHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	if reqHdr := r.Header.Get("Access-Control-Request-Headers"); reqHdr != "" {
		w.Header().Set("Access-Control-Allow-Headers", reqHdr)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Accept")
	}
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
	supported := []int{1, 11}
	if h.Server != nil {
		supported = h.Server.SupportedNIPs()
	}

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
