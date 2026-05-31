package plugin

import "encoding/json"

// Plugin is a relay extension registered at startup.
type Plugin interface {
	Manifest() Manifest
	Init(host HostAPI) error
}

// ConfigValidator validates plugin-owned settings JSON.
type ConfigValidator interface {
	ValidateConfig(settings json.RawMessage) error
}

// Manifest describes a plugin's identity, routing, capabilities, and config schema.
type Manifest struct {
	ID             string
	NIPNumber      int // zero when not a NIP plugin
	URL            string
	Title          string
	Description    string
	DefaultEnabled bool
	Priority       int
	Capabilities   []Capability
	Routes         []Route
	ConfigSchema   ConfigSchema
	// KindClasses maps kind numbers to storage semantics (advisory; core storage uses NIP-01 ranges).
	KindClasses map[int]KindClass
}

// Route declares when a plugin participates in EVENT or REQ handling.
type Route struct {
	Kinds    []int
	TagMatch TagMatch
	CatchAll bool
	Event    *DirectionPolicy
	Req      *DirectionPolicy
}

// DirectionPolicy configures plugin behavior for one pipeline direction.
type DirectionPolicy struct {
	RequiresAuth bool
}

// TagMatch selects events or filters that include a single-letter tag (without "#").
type TagMatch string

// KindClass describes storage and replacement semantics for a kind in a manifest.
type KindClass int

const (
	KindRegular KindClass = iota
	KindReplaceable
	KindEphemeral
	KindAddressable
)
