package position

import (
	"sync"
	"time"

	"order-position-engine/internal/order"
	"order-position-engine/internal/shared"
)

// Manager manages in-memory position state and idempotency checks with thread safety.
type Manager struct {
	mu         sync.RWMutex
	positions  map[string]int
	seenEvents map[string]struct{}
}

// NewManager initializes a new position manager.
func NewManager() *Manager {
	return &Manager{
		positions:  make(map[string]int),
		seenEvents: make(map[string]struct{}),
	}
}

// ProcessEvent processes an incoming order event idempotently and updates net symbol position.
func (m *Manager) ProcessEvent(event shared.OrderEvent) (shared.ProcessResult, error) {
	if err := order.ValidateEvent(&event); err != nil {
		return shared.ProcessResult{
			Status:    shared.StatusRejected,
			EventID:   event.EventID,
			Symbol:    event.Symbol,
			Reason:    err.Error(),
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Idempotency Check: First valid event for an event_id wins.
	if _, exists := m.seenEvents[event.EventID]; exists {
		currPos := m.positions[event.Symbol]
		return shared.ProcessResult{
			Status:      shared.StatusDuplicate,
			EventID:     event.EventID,
			Symbol:      event.Symbol,
			NetPosition: currPos,
			Reason:      "Duplicate event_id ignored",
			Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		}, nil
	}

	// Record event_id into zero-allocation idempotency set
	m.seenEvents[event.EventID] = struct{}{}

	// Update symbol position
	switch event.TransactionType {
	case shared.TxBuy:
		m.positions[event.Symbol] += event.Quantity
	case shared.TxSell:
		m.positions[event.Symbol] -= event.Quantity
	}

	netPos := m.positions[event.Symbol]

	return shared.ProcessResult{
		Status:      shared.StatusAccepted,
		EventID:     event.EventID,
		Symbol:      event.Symbol,
		NetPosition: netPos,
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// GetPositions returns a shallow copy of the net position map safely under RLock.
// Returning a copy prevents concurrent map read & map write panics during JSON encoding.
func (m *Manager) GetPositions() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copyMap := make(map[string]int, len(m.positions))
	for symbol, pos := range m.positions {
		copyMap[symbol] = pos
	}
	return copyMap
}

// Stats returns summary statistics for monitoring telemetry.
func (m *Manager) Stats() (totalSymbols int, uniqueEvents int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.positions), len(m.seenEvents)
}

// Reset clears all in-memory state.
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.positions = make(map[string]int)
	m.seenEvents = make(map[string]struct{})
}
