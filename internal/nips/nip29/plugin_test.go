package nip29

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

type validatorHost struct {
	hostStub
	prefixOK map[string]bool
	restrict bool
	member   bool
	closed   bool
	admins   *nostr.Event
}

func (h *validatorHost) EventIDPrefixExists(_ context.Context, prefix, _ string, _ bool) (bool, error) {
	return h.prefixOK[prefix], nil
}

func (h *validatorHost) GetLatestGroupMetadata39000(context.Context, string) (*nostr.Event, error) {
	if !h.restrict && !h.closed {
		return nil, nil
	}
	md := &nostr.Event{Kind: nostr.NIP29KindGroupMetadata}
	if h.restrict {
		md.Tags = append(md.Tags, []string{"restricted"})
	}
	if h.closed {
		md.Tags = append(md.Tags, []string{"closed"})
	}
	return md, nil
}

func (h *validatorHost) GetLatestGroupAdmins39001(context.Context, string) (*nostr.Event, error) {
	return h.admins, nil
}

func (h *validatorHost) IsGroupMember(context.Context, string, string) (bool, error) {
	return h.member, nil
}

func initValidatorPlugin(t *testing.T, h plugin.HostAPI) *nip29 {
	t.Helper()
	p := New().(*nip29)
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValidatePreviousRejectsUnknownPrefix(t *testing.T) {
	h := &validatorHost{hostStub: hostStub{rpk: stringsRepeat("r", 64)}}
	p := initValidatorPlugin(t, h)
	ev := &nostr.Event{Kind: 1, CreatedAt: time.Now().Unix(), Tags: [][]string{{"h", "g1"}, {"previous", "abcd1234"}}}
	err := p.ValidateEvent(context.Background(), &plugin.EventContext{Event: ev})
	if err == nil {
		t.Fatal("expected rejection for unknown previous prefix")
	}
}

func TestValidatePreviousAcceptsKnownPrefix(t *testing.T) {
	h := &validatorHost{
		hostStub: hostStub{rpk: stringsRepeat("r", 64)},
		prefixOK: map[string]bool{"abcd1234": true},
	}
	p := initValidatorPlugin(t, h)
	ev := &nostr.Event{Kind: 1, CreatedAt: time.Now().Unix(), Tags: [][]string{{"h", "g1"}, {"previous", "abcd1234"}}}
	if err := p.ValidateEvent(context.Background(), &plugin.EventContext{Event: ev}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateLatePublicationRejectsOldEvent(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"late_publication_max_past_seconds": 3600})
	h := &configHost{hostStub: hostStub{rpk: stringsRepeat("r", 64)}, cfg: raw}
	p := initValidatorPlugin(t, h)
	ev := &nostr.Event{
		Kind:      1,
		CreatedAt: time.Now().Unix() - 7200,
		Tags:      [][]string{{"h", "g1"}},
	}
	err := p.ValidateEvent(context.Background(), &plugin.EventContext{Event: ev})
	if err == nil {
		t.Fatal("expected late publication rejection")
	}
}

func TestValidateRestrictedWriteRejectsNonMember(t *testing.T) {
	h := &validatorHost{
		hostStub: hostStub{rpk: stringsRepeat("r", 64)},
		restrict: true,
		member:   false,
	}
	p := initValidatorPlugin(t, h)
	ev := &nostr.Event{
		Kind:   1,
		PubKey: stringsRepeat("u", 64),
		Tags:   [][]string{{"h", "g1"}},
	}
	err := p.ValidateEvent(context.Background(), &plugin.EventContext{Event: ev})
	if err == nil {
		t.Fatal("expected restricted write rejection")
	}
}

func TestValidateJoinRequestRejectsClosedGroup(t *testing.T) {
	h := &validatorHost{
		hostStub: hostStub{rpk: stringsRepeat("r", 64)},
		closed:   true,
		member:   false,
	}
	p := initValidatorPlugin(t, h)
	ev := &nostr.Event{
		Kind:   nostr.NIP29KindJoinRequest,
		PubKey: stringsRepeat("u", 64),
		Tags:   [][]string{{"h", "g1"}},
	}
	err := p.ValidateEvent(context.Background(), &plugin.EventContext{Event: ev})
	if err == nil {
		t.Fatal("expected closed group rejection")
	}
}

func TestManifestCapabilitiesMatchInterfaces(t *testing.T) {
	m := Manifest()
	p := New()
	caps := map[plugin.Capability]struct{}{}
	for _, c := range m.Capabilities {
		caps[c] = struct{}{}
	}
	if _, ok := caps[plugin.CapValidateEvent]; !ok {
		t.Fatal("missing validate_event")
	}
	if _, ok := p.(plugin.EventValidator); !ok {
		t.Fatal("plugin must implement EventValidator")
	}
	if _, ok := p.(plugin.EventStoredHook); !ok {
		t.Fatal("plugin must implement EventStoredHook")
	}
	if _, ok := p.(plugin.ReqVisibility); !ok {
		t.Fatal("plugin must implement ReqVisibility")
	}
	if _, ok := caps[plugin.CapDeleteEvents]; ok {
		t.Fatal("delete_events must not be declared when unused")
	}
}

func TestInitLoadsConfig(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"late_publication_max_past_seconds": 100,
		"strict_previous_same_h":          true,
	})
	h := &configHost{hostStub: hostStub{}, cfg: raw}
	p := New().(*nip29)
	if err := p.Init(h); err != nil {
		t.Fatal(err)
	}
	if p.cfg.LatePublicationMaxPastSeconds != 100 || !p.cfg.StrictPreviousSameH {
		t.Fatalf("config not loaded: %+v", p.cfg)
	}
}

type configHost struct {
	hostStub
	cfg json.RawMessage
}

func (c *configHost) Config() json.RawMessage { return c.cfg }

// Ensure validatorHost satisfies HostAPI via embedding pattern in tests.
var _ plugin.HostAPI = (*validatorHost)(nil)
