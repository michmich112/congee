package plugin

import (
	"reflect"
	"testing"
)

func TestHasCapability(t *testing.T) {
	caps := []Capability{CapReadEvents, CapValidateEvent}
	if !HasCapability(caps, CapReadEvents) {
		t.Fatal("expected read_events")
	}
	if HasCapability(caps, CapWriteEvents) {
		t.Fatal("did not expect write_events")
	}
}

func TestManifestHasCapability(t *testing.T) {
	m := Manifest{Capabilities: []Capability{CapGateReqEvents}}
	if !ManifestHasCapability(m, CapGateReqEvents) {
		t.Fatal("expected gate_req_events")
	}
}

func TestCapabilityFamilies(t *testing.T) {
	host := []Capability{
		CapReadEvents,
		CapWriteEvents,
		CapDeleteEvents,
		CapSignAsRelay,
		CapBroadcast,
	}
	for _, c := range host {
		if !IsHostCapability(c) {
			t.Fatalf("%q should be host capability", c)
		}
		if IsPipelineCapability(c) {
			t.Fatalf("%q should not be pipeline capability", c)
		}
	}

	pipeline := []Capability{
		CapValidateEvent,
		CapReactEvent,
		CapTransformReq,
		CapGateReqEvents,
		CapProvideReqQuery,
	}
	for _, c := range pipeline {
		if !IsPipelineCapability(c) {
			t.Fatalf("%q should be pipeline capability", c)
		}
		if IsHostCapability(c) {
			t.Fatalf("%q should not be host capability", c)
		}
	}
}

func TestHostAndPipelineCapabilitiesFilter(t *testing.T) {
	caps := []Capability{
		CapReadEvents,
		CapValidateEvent,
		CapBroadcast,
		CapTransformReq,
	}
	wantHost := []Capability{CapReadEvents, CapBroadcast}
	wantPipeline := []Capability{CapValidateEvent, CapTransformReq}

	gotHost := HostCapabilities(caps)
	if !reflect.DeepEqual(gotHost, wantHost) {
		t.Fatalf("HostCapabilities = %v, want %v", gotHost, wantHost)
	}

	gotPipeline := PipelineCapabilities(caps)
	if !reflect.DeepEqual(gotPipeline, wantPipeline) {
		t.Fatalf("PipelineCapabilities = %v, want %v", gotPipeline, wantPipeline)
	}
}

func TestHostAndPipelineCapabilitiesPreserveOrder(t *testing.T) {
	caps := []Capability{CapBroadcast, CapReadEvents, CapReactEvent, CapWriteEvents}
	got := HostCapabilities(caps)
	want := []Capability{CapBroadcast, CapReadEvents, CapWriteEvents}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
