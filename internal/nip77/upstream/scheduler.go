package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	"github.com/rs/zerolog"
)

// JobStatus is the last run snapshot for one upstream entry.
type JobStatus struct {
	Name              string `json:"name"`
	URL               string `json:"url"`
	Enabled           bool   `json:"enabled"`
	LastRunUnix       int64  `json:"last_run_unix,omitempty"`
	NextRunUnix       int64  `json:"next_run_unix,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	LastNeedCount     int    `json:"last_need_count,omitempty"`
	LastImported      int    `json:"last_imported,omitempty"`
	LastSkipped       int    `json:"last_skipped,omitempty"`
	LastFailed        int    `json:"last_failed,omitempty"`
	LastFiltersFailed int    `json:"last_filters_failed,omitempty"`
	Running           bool   `json:"running"`
}

type pullStats struct {
	need, imported, skipped, failed, filtersFailed int
}

func (s *pullStats) add(other pullStats) {
	s.need += other.need
	s.imported += other.imported
	s.skipped += other.skipped
	s.failed += other.failed
	s.filtersFailed += other.filtersFailed
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

	stats, err := sch.pullUpstream(ctx, u)
	now := time.Now().Unix()

	sch.mu.Lock()
	st = sch.status[u.Name]
	st.LastRunUnix = now
	st.NextRunUnix = now + int64(u.IntervalSeconds)
	st.LastNeedCount = stats.need
	st.LastImported = stats.imported
	st.LastSkipped = stats.skipped
	st.LastFailed = stats.failed
	st.LastFiltersFailed = stats.filtersFailed
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	sch.mu.Unlock()

	if sch.srv != nil && sch.srv.Metrics() != nil {
		sch.srv.Metrics().IncNegUpstreamImported(int64(stats.imported))
	}
	if err != nil {
		if sch.srv != nil && sch.srv.Metrics() != nil {
			sch.srv.Metrics().IncNegUpstreamFailure()
		}
		audit.Enqueue(storage.AuditEntry{
			CreatedAt: now,
			Action:    audit.ActionNegUpstreamSyncFailed,
			Detail:    fmt.Sprintf("upstream=%s need=%d imported=%d skipped=%d failed=%d filters_failed=%d reason=%s", u.Name, stats.need, stats.imported, stats.skipped, stats.failed, stats.filtersFailed, audit.SanitizeAuditDetailFragment(err.Error())),
		})
		jobLog.Warn().Err(err).Int("need", stats.need).Int("imported", stats.imported).Int("skipped", stats.skipped).Int("failed", stats.failed).Int("filters_failed", stats.filtersFailed).Int64("duration_ms", time.Since(t0).Milliseconds()).Msg("upstream sync failed")
		return
	}
	audit.Enqueue(storage.AuditEntry{
		CreatedAt: now,
		Action:    audit.ActionNegUpstreamSyncComplete,
		Detail:    fmt.Sprintf("upstream=%s need=%d imported=%d skipped=%d failed=%d duration_ms=%d", u.Name, stats.need, stats.imported, stats.skipped, stats.failed, time.Since(t0).Milliseconds()),
	})
	jobLog.Info().Int("need", stats.need).Int("imported", stats.imported).Int("skipped", stats.skipped).Int("failed", stats.failed).Int64("duration_ms", time.Since(t0).Milliseconds()).Msg("upstream sync complete")
}

func (sch *Scheduler) pullUpstream(ctx context.Context, u config.NIP77Upstream) (stats pullStats, err error) {
	log := sch.log.With().Str("upstream", u.Name).Logger()

	filters, err := parseUpstreamFilters(u.Filters)
	if err != nil {
		return stats, err
	}

	log.Debug().Str("url", u.URL).Msg("upstream dialing")
	c, err := dialUpstream(ctx, u.URL)
	if err != nil {
		return stats, err
	}
	defer func() { _ = c.Close() }()
	log.Info().Str("url", u.URL).Int("filters", len(filters)).Msg("upstream connected")

	msgTimeout := time.Duration(config.EffectiveNIP77UpstreamMessageTimeout(sch.cfg)) * time.Second
	authWait := time.Duration(config.EffectiveNIP77UpstreamAuthWait(sch.cfg)) * time.Second
	if authWait > 0 {
		if err := sch.handshakeAuth(ctx, log, c, u.URL, authWait, msgTimeout); err != nil {
			return stats, err
		}
	} else {
		log.Debug().Msg("upstream AUTH wait 0; answering challenges in the message loop")
	}

	frameLimit := config.EffectiveNIP77FrameSizeLimit(sch.cfg)
	var failures []error
	for i, f := range filters {
		log.Debug().Int("filter_index", i).Interface("filter", f).Msg("upstream syncing filter")
		part, err := syncFilter(ctx, sch, log, &c, u.URL, f, frameLimit)
		stats.add(part)
		if err != nil {
			stats.filtersFailed++
			if len(failures) < 5 {
				failures = append(failures, fmt.Errorf("filter %d: %w", i, err))
			}
		}
		log.Debug().Int("filter_index", i).Int("need", part.need).Int("imported", part.imported).Int("failed", part.failed).Msg("upstream filter finished")
	}
	return stats, errors.Join(failures...)
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

func syncFilter(ctx context.Context, sch *Scheduler, log zerolog.Logger, c **wsClient, relayURL string, filter nostr.Filter, frameLimit int) (stats pullStats, err error) {
	local, err := sch.store.QueryEventSyncItems(ctx, filter)
	if err != nil {
		return stats, err
	}
	log.Debug().Int("local_events", len(local)).Msg("upstream local vector built")

	clientNeg := nip77.NewSyncClient(nip77.BuildVector(local), frameLimit)
	defer clientNeg.Stop()
	subID := fmt.Sprintf("up-%d", time.Now().UnixNano())
	initial := clientNeg.Start()

	if err := (*c).sendJSON([]any{"NEG-OPEN", subID, filter, initial}); err != nil {
		return stats, err
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
		typ, payload, err := (*c).readMessage(ctx, msgTimeout)
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
				return stats, fmt.Errorf("negentropy message timeout after %s waiting for NEG-MSG (completed rounds %d)", msgTimeout, round)
			}
			return stats, err
		}
		switch typ {
		case "NEG-ERR":
			var reason string
			if len(payload) >= 3 {
				_ = json.Unmarshal(payload[2], &reason)
			}
			log.Warn().Str("sub_id", subID).Str("reason", reason).Int64("waited_ms", waitMS).Msg("upstream NEG-ERR")
			return stats, fmt.Errorf("upstream neg-err: %s", reason)
		case "NEG-MSG":
			round++
			var msgHex string
			if len(payload) >= 3 {
				_ = json.Unmarshal(payload[2], &msgHex)
			}
			out, err := clientNeg.Reconcile(strings.ToLower(msgHex))
			if err != nil {
				return stats, err
			}
			log.Info().
				Str("sub_id", subID).
				Int("round", round).
				Int("payload_hex_len", len(msgHex)).
				Int64("waited_ms", waitMS).
				Bool("done", out == "").
				Msg("upstream NEG-MSG round")
			if out == "" {
				_ = (*c).sendJSON([]any{"NEG-CLOSE", subID})
				goto fetch
			}
			if err := (*c).sendJSON([]any{"NEG-MSG", subID, out}); err != nil {
				return stats, err
			}
			log.Info().Str("sub_id", subID).Int("round", round).Msg("upstream NEG-MSG reply sent")
		case "AUTH":
			challenge := jsonStringAt(payload, 1)
			log.Info().Str("sub_id", subID).Str("challenge", challenge).Int64("waited_ms", waitMS).Msg("upstream AUTH challenge")
			if err := sch.answerAuth(*c, log, relayURL, challenge); err != nil {
				return stats, err
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
	needIDs := clientNeg.NeedIDs()
	stats.need = len(needIDs)
	sort.Strings(needIDs)
	log.Debug().
		Str("sub_id", subID).
		Int("need", stats.need).
		Int("haves", clientNeg.HaveCount()).
		Int("rounds", round).
		Msg("upstream reconcile complete")

	var failures []error
	for _, id := range needIDs {
		ok, err := sch.fetchPersistWithRetry(ctx, log, c, relayURL, id)
		if err != nil {
			stats.failed++
			if len(failures) < 3 {
				failures = append(failures, err)
			}
			continue
		}
		if !ok {
			stats.skipped++
			continue
		}
		stats.imported++
	}
	if stats.failed > 0 {
		return stats, fmt.Errorf("%d of %d upstream events failed after retries: %w", stats.failed, stats.need, errors.Join(failures...))
	}
	return stats, nil
}

const maxFetchAttempts = 3

// fetchPersistWithRetry retries only one missing event. A new connection is
// required after a read timeout because websocket reads cannot recover from a
// failed deadline. Each attempt also re-verifies the event before storage.
func (sch *Scheduler) fetchPersistWithRetry(ctx context.Context, log zerolog.Logger, c **wsClient, relayURL, id string) (bool, error) {
	var lastErr error
	for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
		if attempt > 1 {
			pause := time.Duration(attempt-1) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(pause):
			}
			next, err := dialUpstream(ctx, relayURL)
			if err != nil {
				lastErr = err
				continue
			}
			authWait := time.Duration(config.EffectiveNIP77UpstreamAuthWait(sch.cfg)) * time.Second
			if authWait > 0 {
				msgTimeout := time.Duration(config.EffectiveNIP77UpstreamMessageTimeout(sch.cfg)) * time.Second
				if err := sch.handshakeAuth(ctx, log, next, relayURL, authWait, msgTimeout); err != nil {
					_ = next.Close()
					lastErr = err
					continue
				}
			}
			_ = (*c).Close()
			*c = next
		}
		fetchTimeout := time.Duration(config.EffectiveNIP77UpstreamMessageTimeout(sch.cfg)) * time.Second
		ev, err := (*c).reqEventByID(ctx, id, fetchTimeout, func(challenge string) error {
			return sch.answerAuth(*c, log, relayURL, challenge)
		})
		if err == nil {
			err = ev.VerifySig()
		}
		if err == nil {
			var stored bool
			stored, err = sch.persistImportedEvent(ctx, ev)
			if err == nil {
				return stored, nil
			}
		}
		lastErr = err
		log.Warn().Str("id", id).Int("attempt", attempt).Err(err).Msg("upstream event import attempt failed")
	}
	return false, fmt.Errorf("event %s: %w", id, lastErr)
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
