package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/michmich112/congee/internal/config"
	nip01manifest "github.com/michmich112/congee/internal/nips/nip01"
	nip02plugin "github.com/michmich112/congee/internal/nips/nip02"
	nip11manifest "github.com/michmich112/congee/internal/nips/nip11"
	nip29plugin "github.com/michmich112/congee/internal/nips/nip29"
	nip42manifest "github.com/michmich112/congee/internal/nips/nip42"
	nip50plugin "github.com/michmich112/congee/internal/nips/nip50"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

type activePlugin struct {
	id         string
	manifest   plugin.Manifest
	host       plugin.HostAPI
	plugin     plugin.Plugin
	order      int
	validator  plugin.EventValidator
	storedHook plugin.EventStoredHook
	transform  plugin.ReqTransformer
	visibility plugin.ReqVisibility
	queryProv  plugin.ReqQueryProvider
}

// Registry loads plugins and executes pipeline phases.
type Registry struct {
	cfg        *config.Config
	store      storage.Store
	log        zerolog.Logger
	srv        *relay.Server
	coreNIPs   []int
	enabledNIP []int
	plugins    []activePlugin
	eventIdx   *routingIndex
	reqIdx     *routingIndex
}

// RegisterBuiltinPlugins returns all built-in plugin factories.
func RegisterBuiltinPlugins() []plugin.Plugin {
	return []plugin.Plugin{
		nip02plugin.New(),
		nip29plugin.New(),
		nip50plugin.New(),
	}
}

// extraPluginFactories is appended during tests (see SetExtraPluginFactoriesForTest).
var extraPluginFactories func() []plugin.Plugin

// SetExtraPluginFactoriesForTest registers additional plugins for Load/LoadWithExtra.
// Pass nil in test cleanup to reset.
func SetExtraPluginFactoriesForTest(fn func() []plugin.Plugin) {
	extraPluginFactories = fn
}

// LoadWithExtra is like Load but registers additional test-only plugins.
func LoadWithExtra(cfg *config.Config, s *relay.Server, store storage.Store, log zerolog.Logger, extra []plugin.Plugin) error {
	prev := extraPluginFactories
	extraPluginFactories = func() []plugin.Plugin { return extra }
	defer func() { extraPluginFactories = prev }()
	return Load(cfg, s, store, log)
}

// Load registers pipeline components for enabled NIPs and plugins.
func Load(cfg *config.Config, s *relay.Server, store storage.Store, log zerolog.Logger) error {
	reg := &Registry{
		cfg:   cfg,
		store: store,
		log:   log,
		srv:   s,
	}
	if err := reg.loadPlugins(); err != nil {
		return err
	}
	relay.RegisterNIP01(s, store)
	if nipEnabled(cfg, 42) {
		relay.RegisterNIP42(s, store)
	}
	s.SetPluginRunner(reg)
	return nil
}

func nipEnabled(cfg *config.Config, n int) bool {
	if cfg == nil {
		return false
	}
	if n == 42 {
		return cfg.NIP42.Enabled
	}
	if id, ok := config.NIPNumberToPluginID[n]; ok {
		return config.PluginEnabled(cfg, id)
	}
	return false
}

func (reg *Registry) loadPlugins() error {
	reg.coreNIPs = []int{1, 11}
	if reg.cfg != nil && reg.cfg.NIP42.Enabled {
		reg.coreNIPs = append(reg.coreNIPs, 42)
	}

	var routeEntries []routeEntry
	order := 0

	factories := RegisterBuiltinPlugins()
	if extraPluginFactories != nil {
		factories = append(factories, extraPluginFactories()...)
	}
	for _, factory := range factories {
		p := factory
		m := p.Manifest()
		if m.NIPNumber != 0 {
			if !nipEnabled(reg.cfg, m.NIPNumber) {
				continue
			}
		} else if !m.DefaultEnabled {
			continue
		}

		hostCaps := plugin.HostCapabilities(m.Capabilities)
		host := relay.NewGuardedHostAPI(reg.srv, reg.store, m.ID, hostCaps, pluginConfigJSON(reg.cfg, m.ID), reg.log)
		if err := verifyCapabilities(m, p); err != nil {
			return fmt.Errorf("registry: plugin %q: %w", m.ID, err)
		}
		if err := p.Init(host); err != nil {
			return fmt.Errorf("registry: plugin %q init: %w", m.ID, err)
		}

		ap := activePlugin{
			id:       m.ID,
			manifest: m,
			host:     host,
			plugin:   p,
			order:    order,
		}
		if v, ok := p.(plugin.EventValidator); ok && plugin.ManifestHasCapability(m, plugin.CapValidateEvent) {
			ap.validator = v
		}
		if h, ok := p.(plugin.EventStoredHook); ok && plugin.ManifestHasCapability(m, plugin.CapReactEvent) {
			ap.storedHook = h
		}
		if t, ok := p.(plugin.ReqTransformer); ok && plugin.ManifestHasCapability(m, plugin.CapTransformReq) {
			ap.transform = t
		}
		if vis, ok := p.(plugin.ReqVisibility); ok && plugin.ManifestHasCapability(m, plugin.CapGateReqEvents) {
			ap.visibility = vis
		}
		if q, ok := p.(plugin.ReqQueryProvider); ok && plugin.ManifestHasCapability(m, plugin.CapProvideReqQuery) {
			ap.queryProv = q
		}

		reg.plugins = append(reg.plugins, ap)
		if m.NIPNumber != 0 {
			reg.enabledNIP = append(reg.enabledNIP, m.NIPNumber)
		}

		for _, r := range m.Routes {
			routeEntries = append(routeEntries, routeEntry{
				pluginID: m.ID,
				priority: m.Priority,
				order:    ap.order,
				route:    r,
				manifest: m,
			})
		}
		order++
	}

	reg.eventIdx = newRoutingIndex(routeEntries)
	reg.reqIdx = newRoutingIndex(routeEntries)
	_ = nip01manifest.Manifest()
	_ = nip11manifest.Manifest()
	_ = nip42manifest.Manifest()
	_ = nip29plugin.Manifest()
	return nil
}

func pluginConfigJSON(cfg *config.Config, pluginID string) json.RawMessage {
	return config.PluginSettings(cfg, pluginID)
}

func verifyCapabilities(m plugin.Manifest, p plugin.Plugin) error {
	decl := make(map[plugin.Capability]struct{}, len(m.Capabilities))
	for _, c := range m.Capabilities {
		decl[c] = struct{}{}
	}
	check := func(cap plugin.Capability, has bool) error {
		_, declared := decl[cap]
		if has && !declared {
			return fmt.Errorf("implements %s but capability not declared", cap)
		}
		if declared && !has {
			return fmt.Errorf("declares %s but interface not implemented", cap)
		}
		return nil
	}
	if err := check(plugin.CapValidateEvent, implements[plugin.EventValidator](p)); err != nil {
		return err
	}
	if err := check(plugin.CapReactEvent, implements[plugin.EventStoredHook](p)); err != nil {
		return err
	}
	if err := check(plugin.CapTransformReq, implements[plugin.ReqTransformer](p)); err != nil {
		return err
	}
	if err := check(plugin.CapGateReqEvents, implements[plugin.ReqVisibility](p)); err != nil {
		return err
	}
	if err := check(plugin.CapProvideReqQuery, implements[plugin.ReqQueryProvider](p)); err != nil {
		return err
	}
	for _, c := range m.Capabilities {
		if plugin.IsHostCapability(c) {
			continue
		}
		if !plugin.IsPipelineCapability(c) {
			return fmt.Errorf("unknown capability %q", c)
		}
	}
	return nil
}

func implements[T any](p plugin.Plugin) bool {
	_, ok := p.(T)
	return ok
}

func (reg *Registry) pluginByID(id string) (*activePlugin, bool) {
	for i := range reg.plugins {
		if reg.plugins[i].id == id {
			return &reg.plugins[i], true
		}
	}
	return nil, false
}

func (reg *Registry) ValidateEvent(ctx context.Context, ec *plugin.EventContext) error {
	for _, m := range reg.eventIdx.matchEvent(ec.Event) {
		ap, ok := reg.pluginByID(m.pluginID)
		if !ok || ap.validator == nil {
			continue
		}
		if err := ap.validator.ValidateEvent(ctx, ec); err != nil {
			return err
		}
	}
	return nil
}

func (reg *Registry) OnEventStored(ctx context.Context, ec *plugin.EventContext) error {
	for _, m := range reg.eventIdx.matchEvent(ec.Event) {
		ap, ok := reg.pluginByID(m.pluginID)
		if !ok || ap.storedHook == nil {
			continue
		}
		if err := ap.storedHook.OnEventStored(ctx, ec); err != nil {
			return err
		}
	}
	return nil
}

func (reg *Registry) EventRequiresAuth(ctx context.Context, ec *plugin.EventContext) bool {
	_ = ctx
	return eventRequiresAuth(reg.eventIdx.matchEvent(ec.Event), ec)
}

func (reg *Registry) ReqRequiresAuth(ctx context.Context, conn plugin.ConnInfo, filters []nostr.Filter) bool {
	_ = ctx
	return reqRequiresAuthForFilters(reg.reqIdx, conn, filters)
}

func (reg *Registry) PrepareReq(ctx context.Context, conn plugin.ConnInfo, subID string, filters []nostr.Filter) ([]relay.SubFilterGate, error) {
	gates := make([]relay.SubFilterGate, len(filters))
	for i := range filters {
		original := filters[i]
		current := relay.CloneFilter(original)
		rc := &plugin.ReqContext{
			Conn:    conn,
			SubID:   subID,
			Filters: []nostr.Filter{current},
			Values:  make(map[string]any),
		}

		var visGates []relay.VisibilityGate
		for _, m := range reg.reqIdx.matchFilter(&original) {
			ap, ok := reg.pluginByID(m.pluginID)
			if !ok {
				continue
			}
			if ap.transform != nil {
				before := relay.CloneFilter(current)
				if err := ap.transform.TransformReq(ctx, rc); err != nil {
					return nil, err
				}
				if len(rc.Filters) > 0 {
					if relay.FilterSubset(original, rc.Filters[0]) {
						current = rc.Filters[0]
					} else {
						current = before
						rc.Filters[0] = current
					}
				}
			}
			if ap.visibility != nil {
				snap := *rc
				snap.Filters = []nostr.Filter{relay.CloneFilter(current)}
				visGates = append(visGates, relay.VisibilityGate{
					PluginID:   ap.id,
					ReqContext: snap,
					Visible:    ap.visibility,
				})
			}
		}
		gates[i] = relay.SubFilterGate{Filter: current, Gates: visGates}
	}
	return gates, nil
}

func (reg *Registry) QueryInitialFilter(ctx context.Context, gate relay.SubFilterGate) ([]*nostr.Event, error) {
	f := gate.Filter
	for _, m := range reg.reqIdx.matchFilter(&f) {
		ap, ok := reg.pluginByID(m.pluginID)
		if !ok || ap.queryProv == nil {
			continue
		}
		rc := &plugin.ReqContext{Filters: []nostr.Filter{f}, Values: make(map[string]any)}
		evs, handled, err := ap.queryProv.QueryReq(ctx, rc)
		if err != nil {
			return nil, err
		}
		if handled {
			return evs, nil
		}
	}
	return reg.store.QueryEvents(ctx, []nostr.Filter{f})
}

func (reg *Registry) EventVisible(ctx context.Context, gate relay.SubFilterGate, ev *nostr.Event) (bool, error) {
	for _, vg := range gate.Gates {
		ok, err := vg.Visible.EventVisible(ctx, &vg.ReqContext, ev)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

func (reg *Registry) SupportedNIPs() []int {
	out := append([]int(nil), reg.coreNIPs...)
	out = append(out, reg.enabledNIP...)
	slices.Sort(out)
	return slices.Compact(out)
}
