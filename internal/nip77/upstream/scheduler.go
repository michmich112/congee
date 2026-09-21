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
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
	"github.com/nbd-wtf/go-nostr/nip77/negentropy"
	"github.com/rs/zerolog"
)

// JobStatus is the last run snapshot for one upstream entry.
type JobStatus struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Enabled       bool   `json:"enabled"`
	LastRunUnix   int64  `json:"last_run_unix,omitempty"`
	NextRunUnix   int64  `json:"next_run_unix,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastNeedCount int    `json:"last_need_count,omitempty"`
	LastImported  int    `json:"last_imported,omitempty"`
	Running       bool   `json:"running"`
}

// Scheduler runs configured upstream pull jobs.
type Scheduler struct {
	cfg    *config.Config
	store  storage.Store
	srv    *relay.Server
	id     *relayidentity.Identity
	log    zerolog.Logger
	cancel context.CancelFunc

	mu      sync.RWMutex
	status  map[string]*JobStatus
	running map[string]bool
}

// NewScheduler constructs an upstream sync scheduler.
func NewScheduler(cfg *config.Config, store storage.Store, srv *relay.Server, id *relayidentity.Identity, log zerolog.Logger) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		store:   store,
		srv:     srv,
		id:      id,
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
	log := sch.log.With().Str("upstream", u.Name).Logger()

	filters, err := parseUpstreamFilters(u.Filters)
	if err != nil {
		return 0, 0, err
	}

	log.Debug().Str("url", u.URL).Msg("upstream dialing")
	c, err := dialUpstream(ctx, u.URL)
	if err != nil {
		return 0, 0, err
	}
	defer c.Close()
	log.Info().Str("url", u.URL).Int("filters", len(filters)).Msg("upstream connected")

	msgTimeout := time.Duration(config.EffectiveNIP77UpstreamMessageTimeout(sch.cfg)) * time.Second
	authWait := time.Duration(config.EffectiveNIP77UpstreamAuthWait(sch.cfg)) * time.Second
	if authWait > 0 {
		if err := sch.handshakeAuth(ctx, log, c, u.URL, authWait, msgTimeout); err != nil {
			return 0, 0, err
		}
	} else {
		log.Debug().Msg("upstream AUTH wait 0; answering challenges in the message loop")
	}

	frameLimit := config.EffectiveNIP77FrameSizeLimit(sch.cfg)
	for i, f := range filters {
		log.Debug().Int("filter_index", i).Interface("filter", f).Msg("upstream syncing filter")
		need, imp, err := syncFilter(ctx, sch, log, c, u.URL, f, frameLimit)
		if err != nil {
			return needTotal, imported, err
		}
		log.Debug().Int("filter_index", i).Int("need", need).Int("imported", imp).Msg("upstream filter synced")
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

func syncFilter(ctx context.Context, sch *Scheduler, log zerolog.Logger, c *wsClient, relayURL string, filter nostr.Filter, frameLimit int) (needCount, imported int, err error) {
	local, err := sch.store.QueryEventSyncItems(ctx, filter)
	if err != nil {
		return 0, 0, err
	}
	log.Debug().Int("local_events", len(local)).Msg("upstream local vector built")

	clientNeg := nip77.NewClientNegentropy(nip77.BuildVector(local), frameLimit)
	subID := fmt.Sprintf("up-%d", time.Now().UnixNano())
	initial := clientNeg.Start()

	if err := c.sendJSON([]any{"NEG-OPEN", subID, filter, initial}); err != nil {
		return 0, 0, err
	}
	log.Info().Str("sub_id", subID).Msg("upstream NEG-OPEN sent")

	msgTimeout := time.Duration(config.EffectiveNIP77UpstreamMessageTimeout(sch.cfg)) * time.Second
	round := 0
	for {
		nextRound := round + 1
		log.Info().
			Str("sub_id", subID).
			Int("waiting_for_round", nextRound).
			Int("timeout_seconds", int(msgTimeout.Seconds())).
			Msg("upstream waiting for NEG-MSG")
		waitStart := time.Now()
		typ, payload, err := c.readMessage(ctx, msgTimeout)
		waitMS := time.Since(waitStart).Milliseconds()
		if err != nil {
			if isTimeoutErr(err) {
				log.Warn().
					Err(err).
					Str("sub_id", subID).
					Int("round", round).
					Int("waiting_for_round", nextRound).
					Int("timeout_seconds", int(msgTimeout.Seconds())).
					Int64("waited_ms", waitMS).
					Msg("upstream negentropy message timeout")
				return needCount, imported, fmt.Errorf("negentropy message timeout after %s waiting for NEG-MSG (completed rounds %d)", msgTimeout, round)
			}
			return needCount, imported, err
		}
		switch typ {
		case "NEG-ERR":
			var reason string
			if len(payload) >= 3 {
				_ = json.Unmarshal(payload[2], &reason)
			}
			log.Warn().Str("sub_id", subID).Str("reason", reason).Int64("waited_ms", waitMS).Msg("upstream NEG-ERR")
			return needCount, imported, fmt.Errorf("upstream neg-err: %s", reason)
		case "NEG-MSG":
			round++
			var msgHex string
			if len(payload) >= 3 {
				_ = json.Unmarshal(payload[2], &msgHex)
			}
			out, err := clientNeg.Reconcile(strings.ToLower(msgHex))
			if err != nil {
				return needCount, imported, err
			}
			log.Info().
				Str("sub_id", subID).
				Int("round", round).
				Int("payload_hex_len", len(msgHex)).
				Int64("waited_ms", waitMS).
				Bool("done", out == "").
				Msg("upstream NEG-MSG round")
			if out == "" {
				_ = c.sendJSON([]any{"NEG-CLOSE", subID})
				goto fetch
			}
			if err := c.sendJSON([]any{"NEG-MSG", subID, out}); err != nil {
				return needCount, imported, err
			}
			log.Info().Str("sub_id", subID).Int("round", round).Msg("upstream NEG-MSG reply sent")
		case "AUTH":
			challenge := jsonStringAt(payload, 1)
			log.Info().Str("sub_id", subID).Str("challenge", challenge).Int64("waited_ms", waitMS).Msg("upstream AUTH challenge")
			if err := sch.answerAuth(c, log, relayURL, challenge); err != nil {
				return needCount, imported, err
			}
		case "OK":
			log.Info().
				Str("sub_id", subID).
				Str("event_id", jsonStringAt(payload, 1)).
				Bool("accepted", jsonBoolAt(payload, 2)).
				Str("msg", jsonStringAt(payload, 3)).
				Int64("waited_ms", waitMS).
				Msg("upstream OK")
		case "NOTICE":
			log.Info().Str("sub_id", subID).Str("notice", jsonStringAt(payload, 1)).Int64("waited_ms", waitMS).Msg("upstream NOTICE")
		default:
			log.Info().
				Str("sub_id", subID).
				Str("typ", typ).
				Int64("waited_ms", waitMS).
				Msg("upstream ignored non-negentropy message")
		}
	}

fetch:
	needIDs := collectNeedIDs(clientNeg)
	needCount = len(needIDs)
	log.Debug().Str("sub_id", subID).Int("need", needCount).Int("rounds", round).Msg("upstream reconcile complete")

	for _, id := range needIDs {
		ev, err := c.reqEventByID(ctx, id)
		if err != nil {
			log.Debug().Str("id", id).Err(err).Msg("upstream fetch event failed")
			continue
		}
		if err := ev.VerifySig(); err != nil {
			log.Debug().Str("id", id).Err(err).Msg("upstream event sig invalid")
			continue
		}
		ok, err := sch.persistImportedEvent(ctx, ev)
		if err != nil {
			log.Debug().Str("id", id).Err(err).Msg("upstream save event failed")
			continue
		}
		if !ok {
			continue
		}
		log.Debug().Str("id", id).Int("kind", ev.Kind).Msg("upstream event imported")
		imported++
	}
	return needCount, imported, nil
}

// persistImportedEvent saves a newly fetched upstream event and notifies plugins.
// Returns false when the event was already present. Events are not re-validated
// beyond the caller’s VerifySig — they skip the WebSocket EVENT hook chain.
func (sch *Scheduler) persistImportedEvent(ctx context.Context, ev *nostr.Event) (bool, error) {
	if sch == nil || sch.store == nil || ev == nil {
		return false, nil
	}
	has, err := sch.store.HasEventID(ctx, ev.ID)
	if err != nil {
		return false, err
	}
	if has {
		return false, nil
	}
	if err := sch.store.SaveEvent(ctx, ev); err != nil {
		return false, err
	}
	if sch.srv != nil {
		sch.srv.NotifyPluginStoredEvent(ev, true)
	}
	return true, nil
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
