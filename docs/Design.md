# Low-Level Design (LLD)

This document details the Low-Level Design (LLD), data structures, concurrency synchronization patterns, struct interfaces, and package organization of the platform.

---

## 1. Directory & Package Organization

The repository follows the standard Go project layout:

```text
d:\SDE Intern/
├── cmd/
│   ├── order_service/main.go       # Entry point for CSV Producer
│   └── position_service/main.go    # Entry point for Position Service & HTTP server
├── internal/
│   ├── order/                      # Producer Domain
│   │   ├── config.go               # Configuration struct & StreamStats
│   │   ├── csv_reader.go           # Incremental CSV reader & Streamer
│   │   ├── validator.go            # Validation logic
│   │   ├── validator_test.go       # Validator unit tests
│   │   ├── throttler.go            # Rate limiter
│   │   └── sender.go               # HTTP client sender
│   ├── position/                   # Consumer Domain
│   │   ├── manager.go              # Thread-safe in-memory state & RLock idempotency
│   │   ├── manager_test.go         # Manager unit tests & concurrency tests
│   │   └── handler.go              # HTTP route handlers (/events, /position, /events/stream)
│   └── shared/                     # Shared Types
│       └── models.go               # OrderEvent, ProcessResult, & TransactionType enums
```

---

## 2. Core Data Structures

### A. Position Storage (`map[string]int`)
- Keys are ticker symbol strings (e.g. `"RELIANCE"`, `"INFY"`).
- Values are net position integer quantities.
- Retains symbols in the map even when net position reaches zero or negative values.

### B. Idempotency Set (`map[string]struct{}`)
- Keys are unique `event_id` strings (e.g. `"evt-0001"`).
- In Go, `struct{}` occupies **0 bytes of memory**, making this the most memory-efficient set implementation possible.

---

## 3. Concurrency & Synchronization Design

Position state operations are protected using `sync.RWMutex`:

```go
type Manager struct {
    mu         sync.RWMutex
    positions  map[string]int
    seenEvents map[string]struct{}
}
```

### Mutex Locking Rules:
1. **Write Operations (`ProcessEvent`)**:
   - Acquires `m.mu.Lock()` (Exclusive Write Lock).
   - Verifies if `event_id` exists in `seenEvents`. If present, returns `StatusDuplicate`.
   - Inserts `event_id` into `seenEvents`.
   - Updates `positions[symbol]` (`+= qty` for `BUY`, `-= qty` for `SELL`).
   - Releases lock via `defer m.mu.Unlock()`.

2. **Read Operations (`GetPositions`)**:
   - Acquires `m.mu.RLock()` (Shared Read Lock).
   - Constructs a **shallow copy** of the `positions` map:
     ```go
     copyMap := make(map[string]int, len(m.positions))
     for symbol, pos := range m.positions {
         copyMap[symbol] = pos
     }
     ```
   - Releases lock via `defer m.mu.RUnlock()`.
   - **Why Shallow Copy?** Exporting a direct reference to a live Go map while another goroutine writes to it causes a fatal runtime crash:
     `fatal error: concurrent map read and map write`
     Returning a shallow copy eliminates panics without blocking concurrent HTTP reads.

---

## 4. HTTP Handler Dependency Injection

HTTP handlers use dependency injection to decouple routing from internal state:

```go
type HandlerDependencies struct {
    Manager      *Manager
    Broadcaster  *SSEBroadcaster
    WebDir       string
    AddAuditLog  func(shared.ProcessResult)
    GetAuditLogs func() []shared.ProcessResult
}
```
Endpoints (`/events`, `/position`, `/events/stream`) are initialized using constructor functions (`NewEventsHandler`, `NewPositionHandler`, `NewStreamHandler`).
