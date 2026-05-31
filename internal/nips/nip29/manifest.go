package nip29

import (
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

// GroupKinds returns NIP-29 moderation, join/leave, and metadata kinds handled by this plugin.
func GroupKinds() []int {
	kinds := make([]int, 0, 25)
	for k := 9000; k <= 9022; k++ {
		kinds = append(kinds, k)
	}
	kinds = append(kinds, nostr.NIP29KindGroupMetadata, nostr.NIP29KindGroupAdmins)
	return kinds
}

func kindClasses() map[int]plugin.KindClass {
	return map[int]plugin.KindClass{
		nostr.NIP29KindGroupMetadata: plugin.KindAddressable,
		nostr.NIP29KindGroupAdmins:   plugin.KindAddressable,
	}
}

// Manifest describes the NIP-29 relay-based groups plugin.
func Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "nip-29",
		NIPNumber:      29,
		Title:          "Relay-based groups",
		Description:    "Group moderation, membership, metadata, and private read gating",
		DefaultEnabled: false,
		Priority:       29,
		Capabilities: []plugin.Capability{
			plugin.CapReadEvents,
			plugin.CapWriteEvents,
			plugin.CapSignAsRelay,
			plugin.CapBroadcast,
			plugin.CapValidateEvent,
			plugin.CapReactEvent,
			plugin.CapGateReqEvents,
		},
		Routes: []plugin.Route{
			{
				TagMatch: "h",
				Event:    &plugin.DirectionPolicy{},
			},
			{
				Kinds: GroupKinds(),
				Event: &plugin.DirectionPolicy{},
			},
			{
				TagMatch: "h",
				Req:      &plugin.DirectionPolicy{},
			},
			{
				Kinds: GroupKinds(),
				Req: &plugin.DirectionPolicy{},
			},
			{
				CatchAll: true,
				Req:      &plugin.DirectionPolicy{},
			},
		},
		KindClasses: kindClasses(),
		ConfigSchema: nip29ConfigSchema(),
	}
}

func nip29ConfigSchema() plugin.ConfigSchema {
	minZero := 0
	return plugin.ConfigSchema{
		Fields: []plugin.ConfigField{
			{
				Key:         "late_publication_max_past_seconds",
				Type:        plugin.ConfigTypeInt,
				Label:       "Late publication max past (seconds)",
				Description: "Reject group events whose created_at is farther in the past than this value. 0 uses the relay default (86400).",
				Default:     0,
				Validation:  plugin.ConfigFieldValidation{Min: &minZero},
			},
			{
				Key:         "strict_previous_same_h",
				Type:        plugin.ConfigTypeBool,
				Label:       "Strict previous same h",
				Description: "Require each previous id prefix to resolve to an event with the same group h tag.",
				Default:     false,
			},
		},
	}
}
