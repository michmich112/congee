package relay

import (
	"errors"
	"fmt"
	"sync"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
)

var (
	// ErrSubscriptionIDTooLong is returned when sub id exceeds configured max.
	ErrSubscriptionIDTooLong = errors.New("relay: subscription id too long")
	// ErrTooManySubscriptions is returned when a connection exceeds its sub cap.
	ErrTooManySubscriptions = errors.New("relay: too many subscriptions")
	// ErrTooManyFilters is returned when a REQ carries too many filters.
	ErrTooManyFilters = errors.New("relay: too many filters per REQ")
)

// SubscriptionManager tracks REQ subscriptions per connection and broadcasts events.
type SubscriptionManager struct {
	mu sync.RWMutex
	// connID -> subID -> filters
	subs map[string]map[string][]nostr.Filter
	// connID -> enqueue outbound JSON
	senders map[string]func([]byte) bool

	maxSubsPerConn int
	maxSubIDLen    int
	maxFilters     int
}

// NewSubscriptionManager builds a manager from relay config slices.
func NewSubscriptionManager(cfg *config.Config) *SubscriptionManager {
	return &SubscriptionManager{
		subs:            make(map[string]map[string][]nostr.Filter),
		senders:         make(map[string]func([]byte) bool),
		maxSubsPerConn:  cfg.ConnectionLimits.MaxSubscriptionsPerConnection,
		maxSubIDLen:     cfg.MaxSubscriptionIDLength,
		maxFilters:      cfg.ConnectionLimits.MaxFiltersPerReq,
	}
}

// RegisterSender records how to deliver EVENT/EOSE/CLOSED to a connection.
func (m *SubscriptionManager) RegisterSender(connID string, send func([]byte) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.senders[connID] = send
}

// UnregisterSender removes send routing and all subs for connID.
func (m *SubscriptionManager) UnregisterSender(connID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.senders, connID)
	cmap := m.subs[connID]
	delete(m.subs, connID)
	if cmap == nil {
		return nil
	}
	out := make([]string, 0, len(cmap))
	for id := range cmap {
		out = append(out, id)
	}
	return out
}

// Add creates or replaces a subscription for connID.
func (m *SubscriptionManager) Add(connID, subID string, filters []nostr.Filter) error {
	if len(subID) > m.maxSubIDLen {
		return fmt.Errorf("%w: len %d max %d", ErrSubscriptionIDTooLong, len(subID), m.maxSubIDLen)
	}
	if len(filters) > m.maxFilters {
		return fmt.Errorf("%w: got %d max %d", ErrTooManyFilters, len(filters), m.maxFilters)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cmap := m.subs[connID]
	if cmap == nil {
		cmap = make(map[string][]nostr.Filter)
		m.subs[connID] = cmap
	}
	if _, exists := cmap[subID]; !exists && len(cmap) >= m.maxSubsPerConn {
		return ErrTooManySubscriptions
	}
	cmap[subID] = filters
	return nil
}

// Remove drops one subscription.
func (m *SubscriptionManager) Remove(connID, subID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cmap := m.subs[connID]; cmap != nil {
		delete(cmap, subID)
		if len(cmap) == 0 {
			delete(m.subs, connID)
		}
	}
}

// TotalSubscriptions returns the number of open REQ subscriptions across all connections.
func (m *SubscriptionManager) TotalSubscriptions() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, cmap := range m.subs {
		n += len(cmap)
	}
	return n
}

// filtersMatch is true if any filter matches the event (NIP-01 OR semantics).
func filtersMatch(filters []nostr.Filter, ev *nostr.Event) bool {
	for i := range filters {
		if filters[i].Matches(ev) {
			return true
		}
	}
	return false
}

// Broadcast delivers EVENT to every matching subscription.
// If visible is nil, all connections receive matching events. Otherwise visible(connID, ev) must be true.
func (m *SubscriptionManager) Broadcast(ev *nostr.Event, visible func(connID string, ev *nostr.Event) bool) {
	if visible == nil {
		visible = func(string, *nostr.Event) bool { return true }
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for connID, cmap := range m.subs {
		send := m.senders[connID]
		if send == nil {
			continue
		}
		if !visible(connID, ev) {
			continue
		}
		for subID, filters := range cmap {
			if !filtersMatch(filters, ev) {
				continue
			}
			b, err := nostr.MarshalRelayEvent(subID, ev)
			if err != nil {
				continue
			}
			send(b)
		}
	}
}

// SubCount returns how many active subscriptions a connection has (for tests).
func (m *SubscriptionManager) SubCount(connID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subs[connID])
}
