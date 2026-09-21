package plugin

import (
	"context"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
)

const interceptLogQueueSize = 256

// InterceptLogEntry is one recorded intercept decision for admin UI.
type InterceptLogEntry struct {
	UnixMilli           int64          `json:"unix_milli"`
	DurationMs          int64          `json:"duration_ms"`
	SubID               string         `json:"sub_id"`
	Filters             []nostr.Filter `json:"filters"`
	Action              string         `json:"action"`
	EventIDs            []string       `json:"event_ids,omitempty"`
	ReshapeFilters      []nostr.Filter `json:"reshape_filters,omitempty"`
	SubscriptionFilters []nostr.Filter `json:"subscription_filters,omitempty"`
	Error               string         `json:"error,omitempty"`
}

// InterceptLogSnapshot is the admin JSON for one plugin's intercept log.
type InterceptLogSnapshot struct {
	Limit   int                 `json:"limit"`
	Dropped int64               `json:"dropped"`
	Entries []InterceptLogEntry `json:"entries"`
}

type interceptLogJob struct {
	pluginID string
	entry    InterceptLogEntry
}

func interceptActionName(a InterceptAction) string {
	switch a {
	case InterceptReshapeREQ:
		return "reshape_req"
	case InterceptRespond:
		return "respond"
	default:
		return "passthrough"
	}
}

func (m *Manager) startInterceptLogWorker(ctx context.Context) {
	if m == nil {
		return
	}
	m.interceptLogMu.Lock()
	if m.interceptLogCh != nil {
		m.interceptLogMu.Unlock()
		return
	}
	m.interceptLogCh = make(chan interceptLogJob, interceptLogQueueSize)
	if m.interceptLogs == nil {
		m.interceptLogs = map[string][]InterceptLogEntry{}
	}
	ch := m.interceptLogCh
	m.interceptLogMu.Unlock()
	go m.runInterceptLogWorker(ctx, ch)
}

// StartInterceptLogWorker starts the async intercept log goroutine without opening host.sock.
func (m *Manager) StartInterceptLogWorker(ctx context.Context) {
	m.startInterceptLogWorker(ctx)
}

func (m *Manager) runInterceptLogWorker(ctx context.Context, ch <-chan interceptLogJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-ch:
			if !ok {
				return
			}
			m.appendInterceptLog(job.pluginID, job.entry)
		}
	}
}

func (m *Manager) enqueueInterceptLog(pluginID string, req *nostr.ReqMessage, res InterceptResult, d time.Duration, fail string) {
	if m == nil || pluginID == "" {
		return
	}
	if m.interceptLogLimit() <= 0 {
		return
	}
	entry := InterceptLogEntry{
		UnixMilli:  time.Now().UnixMilli(),
		DurationMs: d.Milliseconds(),
		Action:     interceptActionName(res.Action),
		Error:      fail,
	}
	if req != nil {
		entry.SubID = req.SubID
		entry.Filters = cloneFilters(req.Filters)
	}
	if len(res.EventIDs) > 0 {
		entry.EventIDs = append([]string(nil), res.EventIDs...)
	}
	if res.Action == InterceptReshapeREQ && len(res.Filters) > 0 {
		entry.ReshapeFilters = cloneFilters(res.Filters)
	}
	if len(res.SubscriptionFilters) > 0 {
		entry.SubscriptionFilters = cloneFilters(res.SubscriptionFilters)
	}
	m.interceptLogMu.Lock()
	ch := m.interceptLogCh
	m.interceptLogMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- interceptLogJob{pluginID: pluginID, entry: entry}:
	default:
		m.interceptLogDropped.Add(1)
	}
}

// RecordInterceptLog injects a completed intercept decision. Never blocks. Used by tests.
func (m *Manager) RecordInterceptLog(pluginID string, req *nostr.ReqMessage, res InterceptResult, fail string) {
	m.enqueueInterceptLog(pluginID, req, res, 0, fail)
}

func (m *Manager) interceptLogLimit() int {
	if m == nil {
		return config.DefaultPluginInterceptLogSize
	}
	n := int(m.interceptLogLimitN.Load())
	if n < 0 {
		return 0
	}
	if n > config.MaxPluginInterceptLogSize {
		return config.MaxPluginInterceptLogSize
	}
	return n
}

func (m *Manager) appendInterceptLog(pluginID string, entry InterceptLogEntry) {
	limit := m.interceptLogLimit()
	m.interceptLogMu.Lock()
	defer m.interceptLogMu.Unlock()
	if limit <= 0 {
		delete(m.interceptLogs, pluginID)
		return
	}
	buf := append(m.interceptLogs[pluginID], entry)
	if len(buf) > limit {
		buf = append([]InterceptLogEntry(nil), buf[len(buf)-limit:]...)
	}
	if m.interceptLogs == nil {
		m.interceptLogs = map[string][]InterceptLogEntry{}
	}
	m.interceptLogs[pluginID] = buf
}

// InterceptLogSnapshot returns newest-first intercept log rows for a plugin.
func (m *Manager) InterceptLogSnapshot(id string) InterceptLogSnapshot {
	out := InterceptLogSnapshot{
		Limit:   config.DefaultPluginInterceptLogSize,
		Dropped: 0,
		Entries: []InterceptLogEntry{},
	}
	if m == nil {
		return out
	}
	out.Limit = m.interceptLogLimit()
	out.Dropped = m.interceptLogDropped.Load()
	m.interceptLogMu.Lock()
	defer m.interceptLogMu.Unlock()
	buf := m.interceptLogs[id]
	if len(buf) == 0 {
		return out
	}
	out.Entries = make([]InterceptLogEntry, len(buf))
	for i := range buf {
		out.Entries[len(buf)-1-i] = buf[i]
	}
	return out
}

// SetInterceptLogLimit updates the host-wide rolling window and trims stored rows.
func (m *Manager) SetInterceptLogLimit(n int) int {
	if m == nil {
		return config.EffectivePluginInterceptLogSize(nil)
	}
	if n < 0 {
		n = 0
	}
	if n > config.MaxPluginInterceptLogSize {
		n = config.MaxPluginInterceptLogSize
	}
	m.mu.Lock()
	if m.cfg != nil {
		v := n
		m.cfg.Plugins.InterceptLogSize = &v
	}
	m.mu.Unlock()
	m.interceptLogLimitN.Store(int64(n))
	m.trimInterceptLogs(n)
	return n
}

func (m *Manager) trimInterceptLogs(limit int) {
	m.interceptLogMu.Lock()
	defer m.interceptLogMu.Unlock()
	if limit <= 0 {
		m.interceptLogs = map[string][]InterceptLogEntry{}
		return
	}
	for id, buf := range m.interceptLogs {
		if len(buf) > limit {
			m.interceptLogs[id] = append([]InterceptLogEntry(nil), buf[len(buf)-limit:]...)
		}
	}
}
