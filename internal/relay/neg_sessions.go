package relay

import (
	"sync"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/nbd-wtf/go-nostr/nip77/negentropy"
)

type negSession struct {
	subID       string
	filter      nostr.Filter
	filterKinds []int
	recordCount int
	openedUnix  int64
	rounds      int
	lastActUnix int64
	neg         *negentropy.Negentropy
	idleCancel  func()
}

type negSessionMap struct {
	mu   sync.Mutex
	byID map[string]*negSession
}

func newNegSessionMap() *negSessionMap {
	return &negSessionMap{byID: make(map[string]*negSession)}
}

func (m *negSessionMap) close(subID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.byID[subID]; ok {
		if s.idleCancel != nil {
			s.idleCancel()
		}
		delete(m.byID, subID)
		return true
	}
	return false
}

func (m *negSessionMap) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.byID {
		if s.idleCancel != nil {
			s.idleCancel()
		}
		delete(m.byID, id)
	}
}

func (m *negSessionMap) set(subID string, s *negSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.byID[subID]; ok && old.idleCancel != nil {
		old.idleCancel()
	}
	m.byID[subID] = s
}

func (m *negSessionMap) get(subID string) (*negSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[subID]
	return s, ok
}

func (m *negSessionMap) remove(subID string) (*negSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[subID]
	if ok {
		if s.idleCancel != nil {
			s.idleCancel()
		}
		delete(m.byID, subID)
	}
	return s, ok
}

// removeIf removes the entry for subID only when it is still the same
// session instance s. This prevents a stale goroutine (idle timer) or a
// reconcile error path from deleting a newer session that reused the subID
// and releasing that session's counter.
func (m *negSessionMap) removeIf(subID string, s *negSession) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.byID[subID]
	if !ok || cur != s {
		return false
	}
	if s.idleCancel != nil {
		s.idleCancel()
	}
	delete(m.byID, subID)
	return true
}

func (m *negSessionMap) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byID)
}

func (m *negSessionMap) auditList() []NegConnAudit {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]NegConnAudit, 0, len(m.byID))
	for subID, s := range m.byID {
		out = append(out, NegConnAudit{
			SubID:           subID,
			OpenedUnix:      s.openedUnix,
			FilterKinds:     append([]int(nil), s.filterKinds...),
			RecordCount:     s.recordCount,
			Rounds:          s.rounds,
			LastActivityUnix: s.lastActUnix,
		})
	}
	return out
}

func (s *negSession) touchActivity() {
	s.lastActUnix = time.Now().Unix()
	s.rounds++
}
