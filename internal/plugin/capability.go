package plugin

// Capability names a host or pipeline permission declared in a manifest.
type Capability string

const (
	CapReadEvents   Capability = "read_events"
	CapWriteEvents  Capability = "write_events"
	CapDeleteEvents Capability = "delete_events"
	CapSignAsRelay  Capability = "sign_as_relay"
	CapBroadcast    Capability = "broadcast"

	CapValidateEvent    Capability = "validate_event"
	CapReactEvent       Capability = "react_event"
	CapTransformReq     Capability = "transform_req"
	CapGateReqEvents    Capability = "gate_req_events"
	CapProvideReqQuery  Capability = "provide_req_query"
)

var hostCapabilities = map[Capability]struct{}{
	CapReadEvents:   {},
	CapWriteEvents:  {},
	CapDeleteEvents: {},
	CapSignAsRelay:  {},
	CapBroadcast:    {},
}

var pipelineCapabilities = map[Capability]struct{}{
	CapValidateEvent:   {},
	CapReactEvent:      {},
	CapTransformReq:    {},
	CapGateReqEvents:   {},
	CapProvideReqQuery: {},
}

// HasCapability reports whether caps includes c.
func HasCapability(caps []Capability, c Capability) bool {
	for _, x := range caps {
		if x == c {
			return true
		}
	}
	return false
}

// ManifestHasCapability reports whether m declares c.
func ManifestHasCapability(m Manifest, c Capability) bool {
	return HasCapability(m.Capabilities, c)
}

// HostCapabilities returns declared host-family capabilities in manifest order.
func HostCapabilities(caps []Capability) []Capability {
	return filterCapabilities(caps, hostCapabilities)
}

// PipelineCapabilities returns declared pipeline-family capabilities in manifest order.
func PipelineCapabilities(caps []Capability) []Capability {
	return filterCapabilities(caps, pipelineCapabilities)
}

// IsHostCapability reports whether c belongs to the host family.
func IsHostCapability(c Capability) bool {
	_, ok := hostCapabilities[c]
	return ok
}

// IsPipelineCapability reports whether c belongs to the pipeline family.
func IsPipelineCapability(c Capability) bool {
	_, ok := pipelineCapabilities[c]
	return ok
}

func filterCapabilities(caps []Capability, family map[Capability]struct{}) []Capability {
	out := make([]Capability, 0, len(caps))
	for _, c := range caps {
		if _, ok := family[c]; ok {
			out = append(out, c)
		}
	}
	return out
}
