package plugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/sdk/plugin/pluginv1"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// Manager owns plugin processes and implements Runtime.
type Manager struct {
	cfg     *config.Config
	cfgPath string
	store   storage.Store
	log     zerolog.Logger
	root    string

	mu      sync.Mutex
	inst    map[string]*instance
	hostSrv *grpc.Server
	hostLis net.Listener
	ctx     context.Context
	cancel  context.CancelFunc

	listenEnqueued atomic.Int64
	listenDropped  atomic.Int64
	interceptN     atomic.Int64
	interceptMs    atomic.Int64

	passthroughNotReady atomic.Int64
	passthroughTimeout  atomic.Int64
	passthroughError    atomic.Int64
}

// Metrics is a snapshot of plugin host counters.
type Metrics struct {
	ListenEnqueued               int64 `json:"listen_enqueued"`
	ListenDropped                int64 `json:"listen_dropped"`
	InterceptN                   int64 `json:"intercept_n"`
	InterceptPassthroughNotReady int64 `json:"plugin_intercept_passthrough_not_ready"`
	InterceptPassthroughTimeout  int64 `json:"plugin_intercept_passthrough_timeout"`
	InterceptPassthroughError    int64 `json:"plugin_intercept_passthrough_error"`
}

func (m *Manager) recordIntercept(d time.Duration, err error) {
	_ = err
	m.interceptN.Add(1)
	m.interceptMs.Add(d.Milliseconds())
}

func (m *Manager) MetricsSnapshot() Metrics {
	return Metrics{
		ListenEnqueued:               m.listenEnqueued.Load(),
		ListenDropped:                m.listenDropped.Load(),
		InterceptN:                   m.interceptN.Load(),
		InterceptPassthroughNotReady: m.passthroughNotReady.Load(),
		InterceptPassthroughTimeout:  m.passthroughTimeout.Load(),
		InterceptPassthroughError:    m.passthroughError.Load(),
	}
}

// Dir resolves the plugins root directory.
func Dir(cfg *config.Config, cfgPath string) string {
	if cfg != nil && cfg.Plugins.Directory != "" {
		return cfg.Plugins.Directory
	}
	if d := os.Getenv("CONGEE_DATA_DIR"); d != "" {
		return filepath.Join(d, "plugins")
	}
	if cfgPath != "" {
		return filepath.Join(filepath.Dir(cfgPath), "plugins")
	}
	return "plugins"
}

// NewManager constructs a plugin manager. Call Start after host.sock is needed.
func NewManager(cfg *config.Config, cfgPath string, store storage.Store, log zerolog.Logger) *Manager {
	root := Dir(cfg, cfgPath)
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if abs, err := filepath.Abs(cfgPath); err == nil {
		cfgPath = abs
	}
	return &Manager{
		cfg:     cfg,
		cfgPath: cfgPath,
		store:   store,
		log:     log.With().Str("component", "plugin").Logger(),
		root:    root,
		inst:    map[string]*instance{},
	}
}

// Start listens on a shared host.sock (under root) and starts enabled plugins.
func (m *Manager) Start(parent context.Context) error {
	m.ctx, m.cancel = context.WithCancel(parent)
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return err
	}
	hostSock := filepath.Join(m.root, "host.sock")
	_ = os.Remove(hostSock)
	lis, err := net.Listen("unix", hostSock)
	if err != nil {
		return fmt.Errorf("plugin host listen: %w", err)
	}
	if err := os.Chmod(hostSock, 0o600); err != nil {
		_ = lis.Close()
		return err
	}
	gs := grpc.NewServer()
	pluginv1.RegisterHostServer(gs, &hostBridge{m: m})
	m.hostSrv = gs
	m.hostLis = lis
	go func() {
		if err := gs.Serve(lis); err != nil {
			m.log.Debug().Err(err).Msg("plugin host grpc stopped")
		}
	}()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg == nil {
		return nil
	}
	for _, item := range m.cfg.Plugins.Items {
		if !item.Enabled {
			continue
		}
		if err := m.startLocked(item); err != nil {
			m.log.Error().Err(err).Str("plugin_id", item.ID).Msg("plugin start")
		}
	}
	return nil
}

func (m *Manager) startLocked(item config.PluginItem) error {
	pkgDir := filepath.Join(m.root, item.ID)
	if abs, err := filepath.Abs(pkgDir); err == nil {
		pkgDir = abs
	}
	man, err := loadManifest(pkgDir)
	if err != nil {
		return err
	}
	dataDir := filepath.Join(pkgDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	in := &instance{
		id:       item.ID,
		item:     item,
		pkgDir:   pkgDir,
		dataDir:  dataDir,
		hostSock: filepath.Join(m.root, "host.sock"),
		man:      man,
		log:      m.log.With().Str("plugin_id", item.ID).Logger(),
		mgr:      m,
		name:     man.Name,
		version:  man.Version,
	}
	m.inst[item.ID] = in
	in.start(m.ctx)
	return nil
}

// Stop stops all plugins and the host socket.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	for _, in := range m.inst {
		in.stop()
	}
	m.inst = map[string]*instance{}
	m.mu.Unlock()
	if m.hostSrv != nil {
		m.hostSrv.GracefulStop()
	}
	if m.hostLis != nil {
		_ = m.hostLis.Close()
	}
}

// Observe implements Runtime.
func (m *Manager) Observe(msg any) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, in := range m.inst {
		if !in.item.Enabled {
			continue
		}
		if MatchObserve(in.subscriptions(), msg) {
			in.enqueueObserve(msg)
		}
	}
}

// InterceptREQ implements Runtime. First non-passthrough wins (config order).
func (m *Manager) InterceptREQ(ctx context.Context, req *nostr.ReqMessage) InterceptResult {
	if m == nil || req == nil {
		return InterceptResult{Action: InterceptPassthrough}
	}
	m.mu.Lock()
	order := make([]*instance, 0, len(m.inst))
	if m.cfg != nil {
		for _, item := range m.cfg.Plugins.Items {
			if in, ok := m.inst[item.ID]; ok {
				order = append(order, in)
			}
		}
	} else {
		for _, in := range m.inst {
			order = append(order, in)
		}
	}
	m.mu.Unlock()
	for _, in := range order {
		if !in.item.Enabled || !MatchInterceptREQ(in.subscriptions(), req.Filters) {
			continue
		}
		res := in.intercept(ctx, req)
		if res.Action != InterceptPassthrough {
			return res
		}
	}
	return InterceptResult{Action: InterceptPassthrough}
}

// EnqueueStoredEvent implements Runtime.
func (m *Manager) EnqueueStoredEvent(ev *nostr.Event, stored bool) {
	if m == nil || ev == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, in := range m.inst {
		if !in.item.Enabled {
			continue
		}
		if MatchOnStoredEvent(in.subscriptions(), ev.Kind) {
			in.enqueueStored(ev, stored)
		}
	}
}

// Snapshot implements Runtime.
func (m *Manager) Snapshot() []InstanceSnapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []InstanceSnapshot
	if m.cfg != nil {
		for _, item := range m.cfg.Plugins.Items {
			if in, ok := m.inst[item.ID]; ok {
				out = append(out, in.snapshot())
				continue
			}
			out = append(out, InstanceSnapshot{ID: item.ID, Enabled: item.Enabled, State: "stopped", Version: item.Version})
		}
		return out
	}
	for _, in := range m.inst {
		out = append(out, in.snapshot())
	}
	return out
}

func (m *Manager) get(id string) *instance {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inst[id]
}

// InstallURL downloads, verifies sha256, and copies the package into the plugins dir.
func (m *Manager) InstallURL(ctx context.Context, url, sha256hex string, enable bool) (*Manifest, error) {
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return nil, err
	}
	man, dest, err := installFromURL(m.root, url, sha256hex)
	if err != nil {
		return nil, err
	}
	m.runInstallHook(ctx, man, dest)
	m.upsertConfigItem(man, url, sha256hex, enable)
	if enable {
		if err := m.Enable(man.ID); err != nil {
			return man, err
		}
	}
	return man, nil
}

// InstallLocal copies a directory that contains plugin.json.
func (m *Manager) InstallLocal(src string, enable bool) (*Manifest, error) {
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return nil, err
	}
	man, dest, err := installFromDir(m.root, src)
	if err != nil {
		return nil, err
	}
	m.runInstallHook(context.Background(), man, dest)
	m.upsertConfigItem(man, src, "", enable)
	if enable {
		if err := m.Enable(man.ID); err != nil {
			return man, err
		}
	}
	return man, nil
}

func (m *Manager) upsertConfigItem(man *Manifest, source, sha string, enabled bool) {
	if m.cfg == nil {
		return
	}
	it := config.PluginItem{ID: man.ID, Enabled: enabled, SourceURL: source, SHA256: sha, Version: man.Version}
	_, idx := config.PluginItemByID(m.cfg, man.ID)
	if idx >= 0 {
		it.Settings = m.cfg.Plugins.Items[idx].Settings
		m.cfg.Plugins.Items[idx] = it
		return
	}
	m.cfg.Plugins.Items = append(m.cfg.Plugins.Items, it)
}

func (m *Manager) runInstallHook(ctx context.Context, man *Manifest, pkgDir string) {
	if man == nil || len(man.Hooks.Install) == 0 {
		return
	}
	dataDir := filepath.Join(pkgDir, "data")
	settings := ""
	if item, idx := config.PluginItemByID(m.cfg, man.ID); idx >= 0 {
		settings = string(item.Settings)
	}
	if err := runManifestHook(ctx, man, pkgDir, dataDir, man.ID, settings, man.Hooks.Install, hookInstallTimeout); err != nil {
		m.log.Warn().Err(err).Str("plugin_id", man.ID).Msg("plugin install hook failed")
	}
}

// Enable starts a plugin if installed.
func (m *Manager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, idx := config.PluginItemByID(m.cfg, id)
	if idx < 0 {
		item = config.PluginItem{ID: id, Enabled: true}
		m.cfg.Plugins.Items = append(m.cfg.Plugins.Items, item)
	} else {
		m.cfg.Plugins.Items[idx].Enabled = true
		item = m.cfg.Plugins.Items[idx]
	}
	if _, ok := m.inst[id]; ok {
		return nil
	}
	return m.startLocked(item)
}

// Disable stops a running plugin without uninstalling.
func (m *Manager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, idx := config.PluginItemByID(m.cfg, id); idx >= 0 {
		m.cfg.Plugins.Items[idx].Enabled = false
	}
	if in, ok := m.inst[id]; ok {
		in.stop()
		delete(m.inst, id)
	}
	return nil
}

// Uninstall stops the plugin, runs hooks.uninstall, then removes the package.
// wipeData deletes data/ (index, secrets) as well as bin/ui; otherwise data/ is kept
// after the uninstall hook has already removed plugin-managed deps (models, runtime libs).
func (m *Manager) Uninstall(id string, wipeData bool) error {
	pkg := filepath.Join(m.root, id)
	if abs, err := filepath.Abs(pkg); err == nil {
		pkg = abs
	}
	dataDir := filepath.Join(pkg, "data")
	man, _ := loadManifest(pkg)
	settings := ""
	if item, idx := config.PluginItemByID(m.cfg, id); idx >= 0 {
		settings = string(item.Settings)
	}

	_ = m.Disable(id)

	if man != nil && len(man.Hooks.Uninstall) > 0 {
		if err := runManifestHook(context.Background(), man, pkg, dataDir, id, settings, man.Hooks.Uninstall, hookUninstallTimeout); err != nil {
			m.log.Warn().Err(err).Str("plugin_id", id).Msg("plugin uninstall hook failed")
		}
	}

	if wipeData {
		if err := os.RemoveAll(pkg); err != nil {
			return err
		}
	} else {
		tmpKeep, err := os.MkdirTemp(m.root, "keep-data-*")
		if err != nil {
			return err
		}
		if err := os.Rename(dataDir, filepath.Join(tmpKeep, "data")); err != nil && !os.IsNotExist(err) {
			_ = os.RemoveAll(tmpKeep)
			return err
		}
		if err := os.RemoveAll(pkg); err != nil {
			_ = os.RemoveAll(tmpKeep)
			return err
		}
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			_ = os.RemoveAll(tmpKeep)
			return err
		}
		_ = os.Rename(filepath.Join(tmpKeep, "data"), filepath.Join(pkg, "data"))
		_ = os.RemoveAll(tmpKeep)
	}

	m.dropConfigItem(id)
	return nil
}

func (m *Manager) dropConfigItem(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg == nil {
		return
	}
	items := m.cfg.Plugins.Items[:0]
	for _, it := range m.cfg.Plugins.Items {
		if it.ID != id {
			items = append(items, it)
		}
	}
	m.cfg.Plugins.Items = items
}

// ApplySettings pushes settings to a running plugin and persists them on the item.
func (m *Manager) ApplySettings(ctx context.Context, id string, settings []byte) error {
	m.mu.Lock()
	if m.cfg != nil {
		if _, idx := config.PluginItemByID(m.cfg, id); idx >= 0 {
			m.cfg.Plugins.Items[idx].Settings = append([]byte(nil), settings...)
		}
	}
	in := m.inst[id]
	m.mu.Unlock()
	if in == nil || !in.isReady() {
		return nil
	}
	in.mu.Lock()
	in.item.Settings = append([]byte(nil), settings...)
	cli := in.client
	in.mu.Unlock()
	if cli == nil {
		return nil
	}
	res, err := cli.ApplySettings(ctx, &pluginv1.ApplySettingsRequest{SettingsJson: string(settings)})
	if err != nil {
		return err
	}
	in.mu.Lock()
	in.subs = subsFromV1(res.GetSubscriptions())
	in.mu.Unlock()
	return nil
}

// AdminAction invokes a plugin action.
func (m *Manager) AdminAction(ctx context.Context, id, name string, payload []byte) ([]byte, error) {
	in := m.get(id)
	if in == nil || !in.isReady() {
		return nil, fmt.Errorf("plugin %s not ready", id)
	}
	in.mu.Lock()
	cli := in.client
	in.mu.Unlock()
	res, err := cli.AdminAction(ctx, &pluginv1.AdminActionRequest{Name: name, PayloadJson: string(payload)})
	if err != nil {
		return nil, err
	}
	return []byte(res.GetPayloadJson()), nil
}

// StatusJSON returns the plugin Status RPC payload.
func (m *Manager) StatusJSON(ctx context.Context, id string) ([]byte, bool, error) {
	in := m.get(id)
	if in == nil || !in.isReady() {
		return []byte(`{}`), false, nil
	}
	in.mu.Lock()
	cli := in.client
	in.mu.Unlock()
	res, err := cli.Status(ctx, &pluginv1.StatusRequest{})
	if err != nil {
		return nil, false, err
	}
	return []byte(res.GetJson()), res.GetReady(), nil
}

// UIDir is the compiled Svelte UI directory for a plugin, if present.
func (m *Manager) UIDir(id string) string {
	return filepath.Join(m.root, id, "ui")
}

// PersistConfig writes cfg to cfgPath if set.
func (m *Manager) PersistConfig() error {
	if m.cfg == nil || m.cfgPath == "" {
		return nil
	}
	return config.WriteConfigAtomic(m.cfgPath, m.cfg)
}

type hostBridge struct {
	pluginv1.UnimplementedHostServer
	m *Manager
}

func (h *hostBridge) QueryEvents(ctx context.Context, req *pluginv1.QueryEventsRequest) (*pluginv1.QueryEventsResponse, error) {
	if h.m.store == nil {
		return &pluginv1.QueryEventsResponse{}, nil
	}
	filters := sdkFiltersToNostr(filtersFromV1(req.GetFilters()))
	evs, err := h.m.store.QueryEvents(ctx, filters)
	if err != nil {
		return nil, err
	}
	out := make([]*pluginv1.Event, 0, len(evs))
	for _, ev := range evs {
		out = append(out, eventToV1(nostrEventToSDK(ev)))
	}
	return &pluginv1.QueryEventsResponse{Events: out}, nil
}

func (h *hostBridge) GetEventsByIDs(ctx context.Context, req *pluginv1.GetEventsByIDsRequest) (*pluginv1.GetEventsByIDsResponse, error) {
	evs, err := GetEventsByIDs(ctx, h.m.store, req.GetIds())
	if err != nil {
		return nil, err
	}
	out := make([]*pluginv1.Event, 0, len(evs))
	for _, ev := range evs {
		out = append(out, eventToV1(nostrEventToSDK(ev)))
	}
	return &pluginv1.GetEventsByIDsResponse{Events: out}, nil
}

func (h *hostBridge) Log(ctx context.Context, req *pluginv1.LogRequest) (*pluginv1.LogResponse, error) {
	_ = ctx
	e := h.m.log.Info()
	switch req.GetLevel() {
	case "debug":
		e = h.m.log.Debug()
	case "warn":
		e = h.m.log.Warn()
	case "error":
		e = h.m.log.Error()
	}
	for k, v := range req.GetFields() {
		e = e.Str(k, v)
	}
	e.Msg(req.GetMessage())
	return &pluginv1.LogResponse{}, nil
}
