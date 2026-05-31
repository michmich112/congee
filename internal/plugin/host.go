package plugin

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

// ErrCapabilityNotGranted is returned when a plugin calls a HostAPI method it did not declare.
var ErrCapabilityNotGranted = errors.New("plugin: capability not granted")

// HostAPI is the narrow surface plugins use instead of relay internals.
type HostAPI interface {
	QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error)
	CountEvents(ctx context.Context, filters []nostr.Filter) (int, error)
	HasEventID(ctx context.Context, id string) (bool, error)
	SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error)
	// NIP-29 scoped store helpers (require read_events).
	EventIDPrefixExists(ctx context.Context, prefix, groupID string, requireSameH bool) (bool, error)
	GetLatestGroupMetadata39000(ctx context.Context, groupID string) (*nostr.Event, error)
	GetLatestGroupAdmins39001(ctx context.Context, groupID string) (*nostr.Event, error)
	IsGroupMember(ctx context.Context, groupID, memberPubkey string) (bool, error)
	SaveEvent(ctx context.Context, ev *nostr.Event) error
	DeleteEvent(ctx context.Context, id string) error
	RelayPubkey() string
	SignAsRelay(ctx context.Context, ev *nostr.Event) error
	Broadcast(ctx context.Context, ev *nostr.Event) error
	Config() json.RawMessage
	Logger() zerolog.Logger
}

// ConfigFieldType is a supported plugin setting type for the admin UI.
type ConfigFieldType string

const (
	ConfigTypeString     ConfigFieldType = "string"
	ConfigTypeInt        ConfigFieldType = "int"
	ConfigTypeBool       ConfigFieldType = "bool"
	ConfigTypeIntList    ConfigFieldType = "int_list"
	ConfigTypeStringList ConfigFieldType = "string_list"
)

// ConfigFieldValidation holds optional validation rules for a config field.
type ConfigFieldValidation struct {
	Min    *int
	Max    *int
	MinLen *int
	MaxLen *int
}

// ConfigField describes one plugin setting for schema-driven admin rendering.
type ConfigField struct {
	Key         string
	Type        ConfigFieldType
	Label       string
	Description string
	Default     any
	Validation  ConfigFieldValidation
}

// ConfigSchema is the typed config shape exported by a plugin manifest.
type ConfigSchema struct {
	Fields []ConfigField
}

// FieldByKey returns the field descriptor for key, or false when absent.
func (s ConfigSchema) FieldByKey(key string) (ConfigField, bool) {
	for _, f := range s.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return ConfigField{}, false
}
