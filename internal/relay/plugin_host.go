package relay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

type serverHostAPI struct {
	s        *Server
	store    storage.Store
	pluginID string
	cfgJSON  json.RawMessage
	log      zerolog.Logger
}

// newServerHostAPI returns an unrestricted HostAPI backed by srv (used to build guarded wrappers).
func newServerHostAPI(s *Server, store storage.Store, pluginID string, cfgJSON json.RawMessage, log zerolog.Logger) *serverHostAPI {
	return &serverHostAPI{
		s:        s,
		store:    store,
		pluginID: pluginID,
		cfgJSON:  cfgJSON,
		log:      log.With().Str("plugin_id", pluginID).Logger(),
	}
}

func (h *serverHostAPI) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	return h.store.QueryEvents(ctx, filters)
}

func (h *serverHostAPI) CountEvents(ctx context.Context, filters []nostr.Filter) (int, error) {
	return h.store.CountEvents(ctx, filters)
}

func (h *serverHostAPI) HasEventID(ctx context.Context, id string) (bool, error) {
	return h.store.HasEventID(ctx, id)
}

func (h *serverHostAPI) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	return h.store.SearchEvents(ctx, searchQuery, constraints)
}

func (h *serverHostAPI) EventIDPrefixExists(ctx context.Context, prefix, groupID string, requireSameH bool) (bool, error) {
	return h.store.EventIDPrefixExists(ctx, prefix, groupID, requireSameH)
}

func (h *serverHostAPI) GetLatestGroupMetadata39000(ctx context.Context, groupID string) (*nostr.Event, error) {
	rpk := h.RelayPubkey()
	if rpk == "" {
		return nil, nil
	}
	return h.store.GetLatestGroupMetadata39000(ctx, rpk, groupID)
}

func (h *serverHostAPI) GetLatestGroupAdmins39001(ctx context.Context, groupID string) (*nostr.Event, error) {
	rpk := h.RelayPubkey()
	if rpk == "" {
		return nil, nil
	}
	return h.store.GetLatestGroupAdmins39001(ctx, rpk, groupID)
}

func (h *serverHostAPI) IsGroupMember(ctx context.Context, groupID, memberPubkey string) (bool, error) {
	rpk := h.RelayPubkey()
	if rpk == "" {
		return false, nil
	}
	return h.store.IsGroupMember(ctx, rpk, groupID, memberPubkey)
}

func (h *serverHostAPI) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	if ev == nil {
		return fmt.Errorf("relay: nil event")
	}
	ctx = withRelayInjected(ctx)
	return h.store.SaveEvent(ctx, ev)
}

func (h *serverHostAPI) DeleteEvent(ctx context.Context, id string) error {
	return h.store.DeleteEvent(ctx, id)
}

func (h *serverHostAPI) RelayPubkey() string {
	if h.s.relayID == nil {
		return ""
	}
	return h.s.relayID.PubKeyHex()
}

func (h *serverHostAPI) SignAsRelay(ctx context.Context, ev *nostr.Event) error {
	_ = ctx
	if h.s.relayID == nil {
		return fmt.Errorf("relay: no relay identity configured")
	}
	if ev == nil {
		return fmt.Errorf("relay: nil event")
	}
	return h.s.relayID.SignEvent(ev)
}

func (h *serverHostAPI) Broadcast(ctx context.Context, ev *nostr.Event) error {
	_ = ctx
	if ev == nil {
		return nil
	}
	h.s.broadcastRelayInjected(ev)
	return nil
}

func (h *serverHostAPI) Config() json.RawMessage {
	if len(h.cfgJSON) == 0 {
		return json.RawMessage(`{}`)
	}
	return h.cfgJSON
}

func (h *serverHostAPI) Logger() zerolog.Logger { return h.log }

// guardedHostAPI wraps HostAPI and enforces declared capabilities.
type guardedHostAPI struct {
	inner    *serverHostAPI
	caps     map[plugin.Capability]struct{}
	pluginID string
	store    storage.Store
	log      zerolog.Logger
}

// NewGuardedHostAPI wraps srv's HostAPI for pluginID, denying undeclared host operations.
func NewGuardedHostAPI(s *Server, store storage.Store, pluginID string, caps []plugin.Capability, cfgJSON json.RawMessage, log zerolog.Logger) plugin.HostAPI {
	set := make(map[plugin.Capability]struct{}, len(caps))
	for _, c := range caps {
		set[c] = struct{}{}
	}
	inner := newServerHostAPI(s, store, pluginID, cfgJSON, log)
	return &guardedHostAPI{
		inner:    inner,
		caps:     set,
		pluginID: pluginID,
		store:    store,
		log:      inner.log,
	}
}

func (g *guardedHostAPI) deny(ctx context.Context, op string) error {
	detail := fmt.Sprintf("plugin_id=%s operation=%s", g.pluginID, op)
	if err := audit.Log(ctx, g.store, audit.ActionPluginCapabilityDenied, detail, ""); err != nil {
		g.log.Error().Err(err).Str("operation", op).Msg("audit save failed for plugin_capability_denied")
	}
	g.log.Warn().Str("operation", op).Msg("plugin host capability denied")
	return plugin.ErrCapabilityNotGranted
}

func (g *guardedHostAPI) check(ctx context.Context, cap plugin.Capability, op string) error {
	if _, ok := g.caps[cap]; ok {
		return nil
	}
	return g.deny(ctx, op)
}

func (g *guardedHostAPI) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "QueryEvents"); err != nil {
		return nil, err
	}
	return g.inner.QueryEvents(ctx, filters)
}

func (g *guardedHostAPI) CountEvents(ctx context.Context, filters []nostr.Filter) (int, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "CountEvents"); err != nil {
		return 0, err
	}
	return g.inner.CountEvents(ctx, filters)
}

func (g *guardedHostAPI) HasEventID(ctx context.Context, id string) (bool, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "HasEventID"); err != nil {
		return false, err
	}
	return g.inner.HasEventID(ctx, id)
}

func (g *guardedHostAPI) SearchEvents(ctx context.Context, searchQuery string, constraints nostr.Filter) ([]*nostr.Event, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "SearchEvents"); err != nil {
		return nil, err
	}
	return g.inner.SearchEvents(ctx, searchQuery, constraints)
}

func (g *guardedHostAPI) EventIDPrefixExists(ctx context.Context, prefix, groupID string, requireSameH bool) (bool, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "EventIDPrefixExists"); err != nil {
		return false, err
	}
	return g.inner.EventIDPrefixExists(ctx, prefix, groupID, requireSameH)
}

func (g *guardedHostAPI) GetLatestGroupMetadata39000(ctx context.Context, groupID string) (*nostr.Event, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "GetLatestGroupMetadata39000"); err != nil {
		return nil, err
	}
	return g.inner.GetLatestGroupMetadata39000(ctx, groupID)
}

func (g *guardedHostAPI) GetLatestGroupAdmins39001(ctx context.Context, groupID string) (*nostr.Event, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "GetLatestGroupAdmins39001"); err != nil {
		return nil, err
	}
	return g.inner.GetLatestGroupAdmins39001(ctx, groupID)
}

func (g *guardedHostAPI) IsGroupMember(ctx context.Context, groupID, memberPubkey string) (bool, error) {
	if err := g.check(ctx, plugin.CapReadEvents, "IsGroupMember"); err != nil {
		return false, err
	}
	return g.inner.IsGroupMember(ctx, groupID, memberPubkey)
}

func (g *guardedHostAPI) SaveEvent(ctx context.Context, ev *nostr.Event) error {
	if err := g.check(ctx, plugin.CapWriteEvents, "SaveEvent"); err != nil {
		return err
	}
	return g.inner.SaveEvent(ctx, ev)
}

func (g *guardedHostAPI) DeleteEvent(ctx context.Context, id string) error {
	if err := g.check(ctx, plugin.CapDeleteEvents, "DeleteEvent"); err != nil {
		return err
	}
	return g.inner.DeleteEvent(ctx, id)
}

func (g *guardedHostAPI) RelayPubkey() string {
	return g.inner.RelayPubkey()
}

func (g *guardedHostAPI) SignAsRelay(ctx context.Context, ev *nostr.Event) error {
	if err := g.check(ctx, plugin.CapSignAsRelay, "SignAsRelay"); err != nil {
		return err
	}
	return g.inner.SignAsRelay(ctx, ev)
}

func (g *guardedHostAPI) Broadcast(ctx context.Context, ev *nostr.Event) error {
	if err := g.check(ctx, plugin.CapBroadcast, "Broadcast"); err != nil {
		return err
	}
	return g.inner.Broadcast(ctx, ev)
}

func (g *guardedHostAPI) Config() json.RawMessage { return g.inner.Config() }

func (g *guardedHostAPI) Logger() zerolog.Logger { return g.inner.Logger() }

// broadcastRelayInjected delivers a host-injected event to subscriptions (REQ visibility only).
func (s *Server) broadcastRelayInjected(ev *nostr.Event) {
	if ev == nil {
		return
	}
	s.subs.Broadcast(ev, s.broadcastSubVisible)
}
