package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	sdk "github.com/michmich112/congee/sdk/plugin"
	"github.com/michmich112/congee/sdk/plugin/pluginv1"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type instanceState int

const (
	stateStarting instanceState = iota
	stateReady
	stateDegraded
	stateStopped
)

const listenQueueSize = 256

type listenJob struct {
	observe *observeJob
	stored  *storedJob
}

type observeJob struct {
	typ     string
	event   *nostr.Event
	subID   string
	filters []nostr.Filter
}

type storedJob struct {
	event  *nostr.Event
	stored bool
}

// InstanceSnapshot is admin JSON for one plugin.
type InstanceSnapshot struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Enabled      bool     `json:"enabled"`
	State        string   `json:"state"`
	Ready        bool     `json:"ready"`
	LastError    string   `json:"last_error,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
	SHA256       string   `json:"sha256,omitempty"`
}

type instance struct {
	id       string
	item     config.PluginItem
	pkgDir   string
	dataDir  string
	hostSock string
	man      *Manifest
	log      zerolog.Logger
	mgr      *Manager

	mu         sync.Mutex
	state      instanceState
	lastErr    string
	subs       []sdk.TrafficSubscription
	caps       []string
	deadline   time.Duration
	name       string
	version    string
	cmd        *exec.Cmd
	pluginConn *grpc.ClientConn
	client     pluginv1.PluginClient
	cancelProc context.CancelFunc

	launchHookDone atomic.Bool

	listenQ chan listenJob
	stopped atomic.Bool

	listenEnqueued atomic.Int64
	listenDropped  atomic.Int64
	interceptN     atomic.Int64
	passthroughN   atomic.Int64
}

func (in *instance) stateName() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	switch in.state {
	case stateReady:
		return "ready"
	case stateDegraded:
		return "degraded"
	case stateStopped:
		return "stopped"
	default:
		return "starting"
	}
}

func (in *instance) snapshot() InstanceSnapshot {
	in.mu.Lock()
	defer in.mu.Unlock()
	st := "starting"
	switch in.state {
	case stateReady:
		st = "ready"
	case stateDegraded:
		st = "degraded"
	case stateStopped:
		st = "stopped"
	}
	return InstanceSnapshot{
		ID:           in.id,
		Name:         in.name,
		Version:      in.version,
		Enabled:      in.item.Enabled,
		State:        st,
		Ready:        in.state == stateReady,
		LastError:    in.lastErr,
		Capabilities: append([]string(nil), in.caps...),
		SourceURL:    in.item.SourceURL,
		SHA256:       in.item.SHA256,
	}
}

func (in *instance) isReady() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.state == stateReady && in.client != nil
}

func (in *instance) subscriptions() []sdk.TrafficSubscription {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]sdk.TrafficSubscription, len(in.subs))
	copy(out, in.subs)
	return out
}

func (in *instance) enqueueObserve(msg any) {
	job := observeJob{}
	switch m := msg.(type) {
	case *nostr.EventMessage:
		job.typ = "EVENT"
		job.event = cloneEvent(&m.Event)
	case *nostr.ReqMessage:
		job.typ = "REQ"
		job.subID = m.SubID
		job.filters = cloneFilters(m.Filters)
	case *nostr.CloseMessage:
		job.typ = "CLOSE"
		job.subID = m.SubID
	case *nostr.AuthMessage:
		job.typ = "AUTH"
		job.event = cloneEvent(&m.Event)
	default:
		return
	}
	in.listenEnqueued.Add(1)
	select {
	case in.listenQ <- listenJob{observe: &job}:
		if in.mgr != nil {
			in.mgr.listenEnqueued.Add(1)
		}
	default:
		in.listenDropped.Add(1)
		if in.mgr != nil {
			in.mgr.listenDropped.Add(1)
		}
	}
}

func (in *instance) enqueueStored(ev *nostr.Event, stored bool) {
	if ev == nil {
		return
	}
	in.listenEnqueued.Add(1)
	select {
	case in.listenQ <- listenJob{stored: &storedJob{event: cloneEvent(ev), stored: stored}}:
		if in.mgr != nil {
			in.mgr.listenEnqueued.Add(1)
		}
	default:
		in.listenDropped.Add(1)
		if in.mgr != nil {
			in.mgr.listenDropped.Add(1)
		}
	}
}

func (in *instance) runListenWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-in.listenQ:
			in.dispatchListen(ctx, job)
		}
	}
}

func (in *instance) dispatchListen(ctx context.Context, job listenJob) {
	if !in.isReady() {
		return
	}
	in.mu.Lock()
	cli := in.client
	in.mu.Unlock()
	if cli == nil {
		return
	}
	timeout := 5 * time.Second
	if job.stored != nil {
		timeout = 30 * time.Second
	}
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if job.observe != nil {
		req := &pluginv1.ObserveRequest{MessageType: job.observe.typ, SubId: job.observe.subID}
		if job.observe.event != nil {
			ev := nostrEventToSDK(job.observe.event)
			req.Event = eventToV1(ev)
		}
		if len(job.observe.filters) > 0 {
			for _, f := range nostrFiltersToSDK(job.observe.filters) {
				req.Filters = append(req.Filters, filterToV1(f))
			}
		}
		if _, err := cli.Observe(wctx, req); err != nil {
			in.log.Debug().Err(err).Msg("plugin observe rpc")
		}
		return
	}
	if job.stored != nil {
		ev := nostrEventToSDK(job.stored.event)
		if _, err := cli.OnStoredEvent(wctx, &pluginv1.OnStoredEventRequest{
			Event:  eventToV1(ev),
			Stored: job.stored.stored,
		}); err != nil {
			in.log.Debug().Err(err).Msg("plugin on_stored rpc")
		}
	}
}

func (in *instance) intercept(ctx context.Context, req *nostr.ReqMessage) InterceptResult {
	if req == nil || !in.isReady() {
		in.passthroughN.Add(1)
		if in.mgr != nil {
			in.mgr.passthroughNotReady.Add(1)
		}
		return InterceptResult{Action: InterceptPassthrough}
	}
	in.mu.Lock()
	cli := in.client
	deadline := in.deadline
	in.mu.Unlock()
	if cli == nil {
		in.passthroughN.Add(1)
		if in.mgr != nil {
			in.mgr.passthroughNotReady.Add(1)
		}
		return InterceptResult{Action: InterceptPassthrough}
	}
	if deadline <= 0 {
		deadline = 80 * time.Millisecond
	}
	ictx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	in.interceptN.Add(1)
	t0 := time.Now()
	res, err := cli.InterceptREQ(ictx, &pluginv1.InterceptREQRequest{
		SubId:   req.SubID,
		Filters: filtersToV1(nostrFiltersToSDK(req.Filters)),
	})
	in.mgr.recordIntercept(time.Since(t0), err)
	if err != nil || res == nil {
		in.passthroughN.Add(1)
		if in.mgr != nil {
			if ictx.Err() != nil {
				in.mgr.passthroughTimeout.Add(1)
			} else {
				in.mgr.passthroughError.Add(1)
			}
		}
		return InterceptResult{Action: InterceptPassthrough}
	}
	switch res.Action {
	case pluginv1.InterceptAction_INTERCEPT_ACTION_RESHAPE_REQ:
		return InterceptResult{Action: InterceptReshapeREQ, Filters: sdkFiltersToNostr(filtersFromV1(res.ReshapeFilters))}
	case pluginv1.InterceptAction_INTERCEPT_ACTION_RESPOND:
		return InterceptResult{
			Action:              InterceptRespond,
			EventIDs:            append([]string(nil), res.EventIds...),
			SubscriptionFilters: sdkFiltersToNostr(filtersFromV1(res.SubscriptionFilters)),
		}
	default:
		in.passthroughN.Add(1)
		return InterceptResult{Action: InterceptPassthrough}
	}
}

func (in *instance) start(parent context.Context) {
	in.listenQ = make(chan listenJob, listenQueueSize)
	go in.runListenWorker(parent)
	go in.loop(parent)
}

func (in *instance) loop(parent context.Context) {
	backoff := 200 * time.Millisecond
	for !in.stopped.Load() {
		if parent.Err() != nil {
			return
		}
		if err := in.spawnAndHandshake(parent); err != nil {
			in.setDegraded(err.Error())
			in.log.Warn().Err(err).Msg("plugin start failed")
			select {
			case <-parent.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 200 * time.Millisecond
		in.waitUntilDead(parent)
		in.teardownProc()
		if in.stopped.Load() || parent.Err() != nil {
			return
		}
		in.setDegraded("socket closed")
	}
}

func (in *instance) spawnAndHandshake(parent context.Context) error {
	in.mu.Lock()
	in.state = stateStarting
	in.mu.Unlock()

	hostSock := in.hostSock
	pluginSock := filepath.Join(in.dataDir, "plugin.sock")
	if hostSock == "" {
		hostSock = filepath.Join(filepath.Dir(in.dataDir), "..", "host.sock")
	}
	if abs, err := filepath.Abs(pluginSock); err == nil {
		pluginSock = abs
	}
	if abs, err := filepath.Abs(hostSock); err == nil {
		hostSock = abs
	}
	_ = os.Remove(pluginSock)

	if err := os.MkdirAll(in.dataDir, 0o755); err != nil {
		return err
	}
	if !in.launchHookDone.Load() && in.man != nil && len(in.man.Hooks.Launch) > 0 {
		if err := runManifestHook(parent, in.man, in.pkgDir, in.dataDir, in.id, string(in.settingsJSON()), in.man.Hooks.Launch, hookLaunchTimeout); err != nil {
			in.log.Warn().Err(err).Msg("plugin launch hook failed")
		}
		in.launchHookDone.Store(true)
	}
	bin, err := in.man.execPath(in.pkgDir)
	if err != nil {
		return err
	}

	pctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(pctx, bin)
	cmd.Dir = in.pkgDir
	cmd.Env = append(os.Environ(),
		sdk.EnvPluginSocket+"="+pluginSock,
		sdk.EnvPluginHostSocket+"="+hostSock,
		sdk.EnvPluginDataDir+"="+in.dataDir,
		sdk.EnvPluginID+"="+in.id,
		sdk.EnvPluginPackageDir+"="+in.pkgDir,
		sdk.EnvPluginSettings+"="+string(in.settingsJSON()),
	)
	cmd.Stdout = newPluginLogWriter(in.log, "stdout")
	cmd.Stderr = newPluginLogWriter(in.log, "stderr")
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	cli, conn, err := waitDialPlugin(pctx, pluginSock, 15*time.Second)
	if err != nil {
		cancel()
		_ = cmd.Process.Kill()
		return err
	}

	hctx, hcancel := context.WithTimeout(pctx, 10*time.Second)
	var hs *pluginv1.HandshakeResponse
	for {
		hs, err = cli.Handshake(hctx, &pluginv1.HandshakeRequest{
			ApiVersion:   sdk.APIVersion,
			SettingsJson: string(in.settingsJSON()),
		})
		if err == nil {
			break
		}
		if hctx.Err() != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	hcancel()
	if err != nil {
		_ = conn.Close()
		cancel()
		_ = cmd.Process.Kill()
		return fmt.Errorf("handshake: %w", err)
	}
	if hs.GetApiVersion() != sdk.APIVersion {
		_ = conn.Close()
		cancel()
		_ = cmd.Process.Kill()
		return fmt.Errorf("api_version %d rejected", hs.GetApiVersion())
	}

	ceiling := config.EffectivePluginInterceptTimeout(in.mgr.cfg)
	hint := time.Duration(hs.GetInterceptDeadlineMs()) * time.Millisecond
	deadline := ceiling
	if hint > 0 && hint < ceiling {
		deadline = hint
	}
	if hint > ceiling {
		deadline = ceiling
	}

	in.mu.Lock()
	in.cmd = cmd
	in.cancelProc = cancel
	in.pluginConn = conn
	in.client = cli
	in.subs = subsFromV1(hs.GetSubscriptions())
	in.caps = append([]string(nil), hs.GetCapabilities()...)
	in.name = hs.GetName()
	in.version = hs.GetVersion()
	in.deadline = deadline
	in.state = stateReady
	in.lastErr = ""
	in.mu.Unlock()
	in.log.Info().Str("plugin_id", in.id).Msg("plugin ready")
	return nil
}

func (in *instance) waitUntilDead(parent context.Context) {
	in.mu.Lock()
	cmd := in.cmd
	conn := in.pluginConn
	in.mu.Unlock()
	done := make(chan struct{})
	if cmd != nil && cmd.Process != nil {
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
	} else {
		close(done)
	}
	health := time.NewTicker(5 * time.Second)
	defer health.Stop()
	for {
		select {
		case <-parent.Done():
			return
		case <-done:
			return
		case <-health.C:
			if conn == nil {
				return
			}
			in.mu.Lock()
			cli := in.client
			in.mu.Unlock()
			if cli == nil {
				return
			}
			hctx, cancel := context.WithTimeout(parent, time.Second)
			_, err := cli.Health(hctx, &pluginv1.HealthRequest{})
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (in *instance) teardownProc() {
	in.mu.Lock()
	if in.state == stateReady {
		in.state = stateDegraded
	}
	if in.cancelProc != nil {
		in.cancelProc()
		in.cancelProc = nil
	}
	if in.pluginConn != nil {
		_ = in.pluginConn.Close()
		in.pluginConn = nil
	}
	in.client = nil
	cmd := in.cmd
	in.cmd = nil
	in.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		_ = cmd.Process.Kill()
	}
}

func (in *instance) stop() {
	in.stopped.Store(true)
	in.mu.Lock()
	in.state = stateStopped
	in.mu.Unlock()
	in.teardownProc()
}

func (in *instance) setDegraded(msg string) {
	in.mu.Lock()
	in.state = stateDegraded
	in.lastErr = msg
	in.mu.Unlock()
}

func (in *instance) settingsJSON() []byte {
	if len(in.item.Settings) == 0 {
		return []byte("{}")
	}
	return in.item.Settings
}

func waitDialPlugin(ctx context.Context, sock string, timeout time.Duration) (pluginv1.PluginClient, *grpc.ClientConn, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if _, err := os.Stat(sock); err == nil {
			conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				last = err
			} else {
				return pluginv1.NewPluginClient(conn), conn, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if last == nil {
		last = fmt.Errorf("timeout waiting for %s", sock)
	}
	return nil, nil, last
}

func eventToV1(e sdk.Event) *pluginv1.Event {
	tags := make([]*pluginv1.Tag, 0, len(e.Tags))
	for _, t := range e.Tags {
		tags = append(tags, &pluginv1.Tag{Values: append([]string(nil), t...)})
	}
	return &pluginv1.Event{
		Id: e.ID, Pubkey: e.PubKey, CreatedAt: e.CreatedAt, Kind: int32(e.Kind),
		Tags: tags, Content: e.Content, Sig: e.Sig,
	}
}

func filterToV1(f sdk.Filter) *pluginv1.Filter {
	kinds := make([]int32, len(f.Kinds))
	for i, k := range f.Kinds {
		kinds[i] = int32(k)
	}
	var limit *int32
	if f.Limit != nil {
		v := int32(*f.Limit)
		limit = &v
	}
	var tfs []*pluginv1.TagFilter
	for n, vals := range f.Tags {
		tfs = append(tfs, &pluginv1.TagFilter{Name: n, Values: append([]string(nil), vals...)})
	}
	return &pluginv1.Filter{
		Ids: f.IDs, Authors: f.Authors, Kinds: kinds, Since: f.Since, Until: f.Until,
		Limit: limit, Search: f.Search, TagFilters: tfs,
	}
}

func filtersToV1(in []sdk.Filter) []*pluginv1.Filter {
	out := make([]*pluginv1.Filter, 0, len(in))
	for _, f := range in {
		out = append(out, filterToV1(f))
	}
	return out
}

func filtersFromV1(in []*pluginv1.Filter) []sdk.Filter {
	out := make([]sdk.Filter, 0, len(in))
	for _, p := range in {
		if p == nil {
			continue
		}
		kinds := make([]int, len(p.Kinds))
		for i, k := range p.Kinds {
			kinds[i] = int(k)
		}
		var limit *int
		if p.Limit != nil {
			v := int(*p.Limit)
			limit = &v
		}
		tags := map[string][]string{}
		for _, tf := range p.TagFilters {
			if tf != nil {
				tags[tf.Name] = append([]string(nil), tf.Values...)
			}
		}
		out = append(out, sdk.Filter{
			IDs: p.Ids, Authors: p.Authors, Kinds: kinds, Since: p.Since, Until: p.Until,
			Limit: limit, Search: p.Search, Tags: tags,
		})
	}
	return out
}

func subsFromV1(in []*pluginv1.TrafficSubscription) []sdk.TrafficSubscription {
	out := make([]sdk.TrafficSubscription, 0, len(in))
	for _, p := range in {
		if p == nil {
			continue
		}
		kinds := make([]int, len(p.Kinds))
		for i, k := range p.Kinds {
			kinds[i] = int(k)
		}
		out = append(out, sdk.TrafficSubscription{
			MessageTypes:  append([]string(nil), p.MessageTypes...),
			Kinds:         kinds,
			ReqHasSearch:  p.ReqHasSearch,
			ReqTagNames:   append([]string(nil), p.ReqTagNames...),
			InterceptREQ:  p.InterceptReq,
			Observe:       p.Observe,
			OnStoredEvent: p.OnStoredEvent,
		})
	}
	return out
}

func newPluginLogWriter(log zerolog.Logger, stream string) *pluginLogWriter {
	return &pluginLogWriter{log: log, stream: stream}
}

type pluginLogWriter struct {
	log    zerolog.Logger
	stream string
}

func (w *pluginLogWriter) Write(p []byte) (int, error) {
	msg := stringsTrimNL(string(p))
	if msg != "" {
		w.log.Info().Str("stream", w.stream).Msg(msg)
	}
	return len(p), nil
}

func stringsTrimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
