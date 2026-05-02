package relay

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// In-memory ring of individual REQ query timings for /api/stats and percentile detail on
// the latest windows. Main history comes from persisted relay_metric_buckets.
const recentQueryRingCap = 8192

// RelayMetrics holds hot-path atomics, a partial-minute buffer for DB flush, and a fixed ring of REQ query latencies.
type RelayMetrics struct {
	// Lifetime (since relay Serve started).
	eventsStoredOK          atomic.Int64
	eventsRejected          atomic.Int64
	eventsEphemeralOK       atomic.Int64
	reqTotal                atomic.Int64
	closeTotal              atomic.Int64
	rateLimitMessages       atomic.Int64
	rateLimitBandwidth      atomic.Int64
	rateLimitEvents         atomic.Int64
	rateLimitReqs           atomic.Int64
	rateLimitNewConnections atomic.Int64
	rateLimitMaxConnections atomic.Int64

	// Current UTC minute partial aggregates (reset on flush).
	curEventsStored   atomic.Int64
	curEventsRejected atomic.Int64
	curReq            atomic.Int64
	curClose          atomic.Int64
	curQueryMsSum     atomic.Int64
	curQueryMsCount   atomic.Int64

	recentMu       sync.Mutex
	recentHead     int
	recentCount    int
	recentUnixMs   [recentQueryRingCap]int64
	recentQueryMs  [recentQueryRingCap]int64
}

// RecentLatencySample is one REQ query latency observation for GET /api/stats JSON.
type RecentLatencySample struct {
	TUnixMs int64 `json:"t_unix_ms"`
	Ms      int64 `json:"ms"`
}

func newRelayMetrics() *RelayMetrics {
	return &RelayMetrics{}
}

func (m *RelayMetrics) IncEventsStoredOK() {
	m.eventsStoredOK.Add(1)
	m.curEventsStored.Add(1)
}

func (m *RelayMetrics) IncEventsRejected() {
	m.eventsRejected.Add(1)
	m.curEventsRejected.Add(1)
}

func (m *RelayMetrics) IncEventsEphemeralOK() {
	m.eventsEphemeralOK.Add(1)
}

func (m *RelayMetrics) IncReq() {
	m.reqTotal.Add(1)
	m.curReq.Add(1)
}

func (m *RelayMetrics) IncClose() {
	m.closeTotal.Add(1)
	m.curClose.Add(1)
}

func (m *RelayMetrics) IncRateLimitMessages()       { m.rateLimitMessages.Add(1) }
func (m *RelayMetrics) IncRateLimitBandwidth()      { m.rateLimitBandwidth.Add(1) }
func (m *RelayMetrics) IncRateLimitEvents()         { m.rateLimitEvents.Add(1) }
func (m *RelayMetrics) IncRateLimitReqs()           { m.rateLimitReqs.Add(1) }
func (m *RelayMetrics) IncRateLimitNewConnections() { m.rateLimitNewConnections.Add(1) }
func (m *RelayMetrics) IncRateLimitMaxConnections() { m.rateLimitMaxConnections.Add(1) }

func (m *RelayMetrics) RecordQueryLatency(d time.Duration) {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	tUnixMs := time.Now().UnixMilli()
	m.curQueryMsSum.Add(ms)
	m.curQueryMsCount.Add(1)

	m.recentMu.Lock()
	defer m.recentMu.Unlock()
	if m.recentCount < recentQueryRingCap {
		pos := (m.recentHead + m.recentCount) % recentQueryRingCap
		m.recentUnixMs[pos] = tUnixMs
		m.recentQueryMs[pos] = ms
		m.recentCount++
		return
	}
	m.recentUnixMs[m.recentHead] = tUnixMs
	m.recentQueryMs[m.recentHead] = ms
	m.recentHead = (m.recentHead + 1) % recentQueryRingCap
}

// RecentLatencySamples returns up to recentQueryRingCap newest samples (oldest first).
func (m *RelayMetrics) RecentLatencySamples() []RecentLatencySample {
	m.recentMu.Lock()
	defer m.recentMu.Unlock()
	if m.recentCount == 0 {
		return nil
	}
	out := make([]RecentLatencySample, m.recentCount)
	for j := 0; j < m.recentCount; j++ {
		idx := (m.recentHead + j) % recentQueryRingCap
		out[j] = RecentLatencySample{TUnixMs: m.recentUnixMs[idx], Ms: m.recentQueryMs[idx]}
	}
	return out
}

// CountersJSON returns cumulative relay_counters for GET /api/stats.
func (m *RelayMetrics) CountersJSON() map[string]any {
	return map[string]any{
		"events_stored_ok":           m.eventsStoredOK.Load(),
		"events_rejected":            m.eventsRejected.Load(),
		"events_ephemeral_ok":        m.eventsEphemeralOK.Load(),
		"req_total":                  m.reqTotal.Load(),
		"close_total":                m.closeTotal.Load(),
		"rate_limit_messages":        m.rateLimitMessages.Load(),
		"rate_limit_bandwidth":       m.rateLimitBandwidth.Load(),
		"rate_limit_events":          m.rateLimitEvents.Load(),
		"rate_limit_reqs":            m.rateLimitReqs.Load(),
		"rate_limit_new_connections": m.rateLimitNewConnections.Load(),
		"rate_limit_max_connections": m.rateLimitMaxConnections.Load(),
	}
}

// PartialMinuteBucket returns the aligned UTC minute start and in-flight aggregates for charts.
func (m *RelayMetrics) PartialMinuteBucket() (startUnix int64, b storage.RelayMetricBucket) {
	now := time.Now().Unix()
	startUnix = (now / 60) * 60
	b = storage.RelayMetricBucket{
		BucketStartUnix: startUnix,
		EventsStored:    m.curEventsStored.Load(),
		EventsRejected:  m.curEventsRejected.Load(),
		ReqCount:        m.curReq.Load(),
		CloseCount:      m.curClose.Load(),
		QueryMsSum:      m.curQueryMsSum.Load(),
		QueryMsCount:    m.curQueryMsCount.Load(),
	}
	return startUnix, b
}

func (m *RelayMetrics) flushCompletedMinute(ctx context.Context, store storage.Store, completedStart int64, subsSnapshot func() int) error {
	subs := int64(0)
	if subsSnapshot != nil {
		subs = int64(subsSnapshot())
	}
	b := storage.RelayMetricBucket{
		BucketStartUnix:   completedStart,
		EventsStored:      m.curEventsStored.Swap(0),
		EventsRejected:    m.curEventsRejected.Swap(0),
		ReqCount:          m.curReq.Swap(0),
		CloseCount:        m.curClose.Swap(0),
		QueryMsSum:        m.curQueryMsSum.Swap(0),
		QueryMsCount:      m.curQueryMsCount.Swap(0),
		SubscriptionsOpen: subs,
	}
	if b.EventsStored == 0 && b.EventsRejected == 0 && b.ReqCount == 0 && b.CloseCount == 0 && b.QueryMsSum == 0 && b.QueryMsCount == 0 && b.SubscriptionsOpen == 0 {
		return nil
	}
	return store.UpsertRelayMetricBucket(ctx, b)
}

func (m *RelayMetrics) purgeOldBuckets(ctx context.Context, store storage.Store, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	cutoff = (cutoff / 60) * 60
	_, _ = store.PurgeRelayMetricBucketsBefore(ctx, cutoff)
}

// StartFlushLoop aligns to UTC minute boundaries, flushes the previous minute, and prunes old buckets.
// subsSnapshot, if non-nil, records open REQ subscription count for the completed minute row.
func (m *RelayMetrics) StartFlushLoop(ctx context.Context, store storage.Store, retentionDays int, log zerolog.Logger, subsSnapshot func() int) {
	go func() {
		d := time.Until(time.Now().Truncate(time.Minute).Add(time.Minute))
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			completed := (time.Now().Unix()/60)*60 - 60
			if err := m.flushCompletedMinute(ctx, store, completed, subsSnapshot); err != nil {
				log.Error().Err(err).Int64("bucket_start_unix", completed).Msg("relay metrics flush failed")
			}
			m.purgeOldBuckets(ctx, store, retentionDays)
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

// FlushOpenMinute writes the current partial UTC minute to storage (best-effort shutdown).
func (m *RelayMetrics) FlushOpenMinute(ctx context.Context, store storage.Store, subsSnapshot func() int) error {
	start := (time.Now().Unix() / 60) * 60
	subs := int64(0)
	if subsSnapshot != nil {
		subs = int64(subsSnapshot())
	}
	b := storage.RelayMetricBucket{
		BucketStartUnix:   start,
		EventsStored:      m.curEventsStored.Swap(0),
		EventsRejected:    m.curEventsRejected.Swap(0),
		ReqCount:          m.curReq.Swap(0),
		CloseCount:        m.curClose.Swap(0),
		QueryMsSum:        m.curQueryMsSum.Swap(0),
		QueryMsCount:      m.curQueryMsCount.Swap(0),
		SubscriptionsOpen: subs,
	}
	if b.EventsStored == 0 && b.EventsRejected == 0 && b.ReqCount == 0 && b.CloseCount == 0 && b.QueryMsSum == 0 && b.QueryMsCount == 0 && b.SubscriptionsOpen == 0 {
		return nil
	}
	return store.UpsertRelayMetricBucket(ctx, b)
}
