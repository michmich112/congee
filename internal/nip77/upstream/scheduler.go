package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nip77"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage"
	"github.com/nbd-wtf/go-nostr/nip77/negentropy"
	"github.com/rs/zerolog"
)

// JobStatus is the last run snapshot for one upstream entry.
type JobStatus struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Enabled        bool   `json:"enabled"`
	LastRunUnix    int64  `json:"last_run_unix,omitempty"`
	NextRunUnix    int64  `json:"next_run_unix,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	LastNeedCount  int    `json:"last_need_count,omitempty"`
	LastImported   int    `json:"last_imported,omitempty"`
	Running        bool   `json:"running"`
}

// Scheduler runs configured upstream pull jobs.
type Scheduler struct {
	cfg    *config.Config
	store  storage.Store
	srv    *relay.Server
	log    zerolog.Logger
	cancel context.CancelFunc

	mu      sync.RWMutex
	status  map[string]*JobStatus
	running map[string]bool
}

// NewScheduler constructs an upstream sync scheduler.
func NewScheduler(cfg *config.Config, store storage.Store, srv *relay.Server, log zerolog.Logger) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		store:   store,
		srv:     srv,
		log:     log.With().Str("component", "nip77-upstream").Logger(),
		status:  make(map[string]*JobStatus),
		running: make(map[string]bool),
	}
}

// Start launches background tickers for enabled upstreams.
func (sch *Scheduler) Start(ctx context.Context) {
	ctx, sch.cancel = context.WithCancel(ctx)
	for _, u := range sch.cfg.NIP77.Upstreams {
		if !u.Enabled {
			continue
		}
		interval := u.IntervalSeconds
		if interval <= 0 {
			interval = 3600
		}
		sch.initStatus(u)
		go sch.loop(ctx, u, time.Duration(interval)*time.Second)
	}
}

// Stop cancels background workers.
func (sch *Scheduler) Stop() {
	if sch.cancel != nil {
		sch.cancel()
	}
}

// Status returns a snapshot of upstream job states.
func (sch *Scheduler) Status() []JobStatus {
	sch.mu.RLock()
	defer sch.mu.RUnlock()
	out := make([]JobStatus, 0, len(sch.status))
	for _, st := range sch.status {
		cp := *st
		out = append(out, cp)
	}
	return out
}

func (sch *Scheduler) initStatus(u config.NIP77Upstream) {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	sch.status[u.Name] = &JobStatus{Name: u.Name, URL: u.URL, Enabled: u.Enabled}
}

func (sch *Scheduler) loop(ctx context.Context, u config.NIP77Upstream, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	sch.runOnce(ctx, u)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sch.runOnce(ctx, u)
		}
	}
}

func (sch *Scheduler) runOnce(ctx context.Context, u config.NIP77Upstream) {
	if !config.NIP77Enabled(sch.cfg) || !sch.cfg.NIP77.UpstreamEnabled || !u.Enabled {
		return
	}
	if sch.cfg.NIP77.UpstreamPauseWhenBusy && sch.srv != nil && sch.srv.RelayBusyForNeg() {
		sch.log.Debug().Str("upstream", u.Name).Msg("upstream sync skipped: relay busy")
		return
	}
	sch.mu.Lock()
	if sch.running[u.Name] {
		sch.mu.Unlock()
		return
	}
	sch.running[u.Name] = true
	st := sch.status[u.Name]
	if st == nil {
		st = &JobStatus{Name: u.Name, URL: u.URL, Enabled: u.Enabled}
		sch.status[u.Name] = st
	}
	st.Running = true
	sch.mu.Unlock()

	defer func() {
		sch.mu.Lock()
		sch.running[u.Name] = false
		if st := sch.status[u.Name]; st != nil {
			st.Running = false
		}
		sch.mu.Unlock()
	}()

	if sch.srv != nil && sch.srv.Metrics() != nil {
		sch.srv.Metrics().IncNegUpstreamJob()
	}

	jobLog := sch.log.With().Str("upstream", u.Name).Logger()
	jobLog.Info().Msg("upstream sync started")
	t0 := time.Now()

	needTotal, imported, err := sch.pullUpstream(ctx, u)
	now := time.Now().Unix()

	sch.mu.Lock()
	st = sch.status[u.Name]
	st.LastRunUnix = now
	st.NextRunUnix = now + int64(u.IntervalSeconds)
	st.LastNeedCount = needTotal
	st.LastImported = imported
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	sch.mu.Unlock()

	if err != nil {
		if sch.srv != nil && sch.srv.Metrics() != nil {
			sch.srv.Metrics().IncNegUpstreamFailure()
		}
		audit.Enqueue(storage.AuditEntry{
			CreatedAt: now,
			Action:    audit.ActionNegUpstreamSyncFailed,
			Detail:    fmt.Sprintf("upstream=%s reason=%s", u.Name, audit.SanitizeAuditDetailFragment(err.Error())),
		})
		jobLog.Warn().Err(err).Int64("duration_ms", time.Since(t0).Milliseconds()).Msg("upstream sync failed")
		return
	}
	if sch.srv != nil && sch.srv.Metrics() != nil {
		sch.srv.Metrics().IncNegUpstreamImported(int64(imported))
	}
	audit.Enqueue(storage.AuditEntry{
		CreatedAt: now,
		Action:    audit.ActionNegUpstreamSyncComplete,
		Detail:    fmt.Sprintf("upstream=%s need=%d imported=%d duration_ms=%d", u.Name, needTotal, imported, time.Since(t0).Milliseconds()),
	})
	jobLog.Info().Int("need", needTotal).Int("imported", imported).Int64("duration_ms", time.Since(t0).Milliseconds()).Msg("upstream sync complete")
}

func (sch *Scheduler) pullUpstream(ctx context.Context, u config.NIP77Upstream) (needTotal, imported int, err error) {
	filters, err := parseUpstreamFilters(u.Filters)
	if err != nil {
		return 0, 0, err
	}
	c, err := dialUpstream(ctx, u.URL)
	if err != nil {
		return 0, 0, err
	}
	defer c.Close()

	frameLimit := config.EffectiveNIP77FrameSizeLimit(sch.cfg)
	for _, f := range filters {
		need, imp, err := syncFilter(ctx, sch, c, f, frameLimit)
		if err != nil {
			return needTotal, imported, err
		}
		needTotal += need
		imported += imp
	}
	return needTotal, imported, nil
}

func parseUpstreamFilters(raw []json.RawMessage) ([]nostr.Filter, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("no filters")
	}
	out := make([]nostr.Filter, 0, len(raw))
	for i, r := range raw {
		var f nostr.Filter
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("filter %d: %w", i, err)
		}
		if f.HasSearch() {
			return nil, fmt.Errorf("filter %d: search not supported", i)
		}
		out = append(out, f)
	}
	return out, nil
}

func syncFilter(ctx context.Context, sch *Scheduler, c *wsClient, filter nostr.Filter, frameLimit int) (needCount, imported int, err error) {
	local, err := sch.store.QueryEventSyncItems(ctx, filter)
	if err != nil {
		return 0, 0, err
	}
	clientNeg := nip77.NewClientNegentropy(nip77.BuildVector(local), frameLimit)
	subID := fmt.Sprintf("up-%d", time.Now().UnixNano())
	initial := clientNeg.Start()

	if err := c.sendJSON([]any{"NEG-OPEN", subID, filter, initial}); err != nil {
		return 0, 0, err
	}

	for {
		typ, payload, err := c.readMessage(ctx)
		if err != nil {
			return needCount, imported, err
		}
		switch typ {
		case "NEG-ERR":
			var reason string
			if len(payload) >= 3 {
				_ = json.Unmarshal(payload[2], &reason)
			}
			return needCount, imported, fmt.Errorf("upstream neg-err: %s", reason)
		case "NEG-MSG":
			var msgHex string
			if len(payload) >= 3 {
				_ = json.Unmarshal(payload[2], &msgHex)
			}
			out, err := clientNeg.Reconcile(strings.ToLower(msgHex))
			if err != nil {
				return needCount, imported, err
			}
			if out == "" {
				_ = c.sendJSON([]any{"NEG-CLOSE", subID})
				goto fetch
			}
			if err := c.sendJSON([]any{"NEG-MSG", subID, out}); err != nil {
				return needCount, imported, err
			}
		}
	}

fetch:
	needIDs := collectNeedIDs(clientNeg)
	needCount = len(needIDs)
	for _, id := range needIDs {
		ev, err := c.reqEventByID(ctx, id)
		if err != nil {
			continue
		}
		if err := ev.VerifySig(); err != nil {
			continue
		}
		has, err := sch.store.HasEventID(ctx, ev.ID)
		if err != nil || has {
			continue
		}
		if err := sch.store.SaveEvent(ctx, ev); err != nil {
			continue
		}
		imported++
	}
	return needCount, imported, nil
}

func collectNeedIDs(neg *negentropy.Negentropy) []string {
	if neg == nil || neg.HaveNots == nil {
		return nil
	}
	var ids []string
	for id := range neg.HaveNots {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
