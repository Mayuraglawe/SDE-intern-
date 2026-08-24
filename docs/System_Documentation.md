# System Documentation: Go Order Position Engine

## Table of Contents
1. [Rules of This Project](#1-rules-of-this-project)
2. [Architecture of This Project](#2-architecture-of-this-project)
3. [Design of This Project](#3-design-of-this-project)
4. [Security of This Project](#4-security-of-this-project)
5. [Review](#5-review)
6. [Phases of This Project](#6-phases-of-this-project)

---

## 1. Rules of This Project
This document outlines the strict rules, constraints, and non-negotiable behaviors required for this project based on the assessment guidelines.

### Functional Rules
- **Separate Processes**: The Order Update Service and Position Maintaining Service must run as entirely separate OS processes.
- **Incremental Processing**: The Order Update Service must read the CSV incrementally using `encoding/csv`. Loading the entire file into memory is strictly prohibited.
- **Order Preservation**: Valid events must be sent to the Position Maintaining Service in the exact order they appear in the CSV.
- **Throttling**: The Order Update Service must emit no more than 50 events per second (configurable).
- **Concurrent Safety**: Position updates (via HTTP POST) and API reads (via HTTP GET) must remain strictly correct if they occur concurrently. Thread-safe state management via `sync.RWMutex` is required.
- **Idempotency**: The first valid event received for an `event_id` *wins*. Subsequent events with the same `event_id` must be ignored completely, even if payload fields differ.
- **Resilience**: Malformed input must never crash either service. Invalid rows must be logged with a clear reason and skipped.

### Data Contract Rules
- `event_id`: Must be a non-empty string.
- `symbol`: Must be a non-empty string. Case and value must be preserved exactly as supplied.
- `transaction_type`: Must be exactly `BUY` or `SELL` (case-sensitive).
- `quantity`: Must be a strictly positive integer (zero, negative numbers, and non-integers are invalid).

### Out of Scope (Strictly Forbidden)
- Database persistence or disk storage for state.
- Authentication, authorization, or TLS/HTTPS security layers.
- Distributed deployment, Kubernetes, or cloud infrastructure orchestration.
- Exactly-once delivery guarantees across service restarts.
- External message brokers (e.g., Kafka, RabbitMQ).

---

## 2. Architecture of This Project

### System Overview
The system follows a simple, decoupled microservices architecture designed for streaming data processing, built entirely on Go's standard library.

```
                         +-----------------------------------+
                         |      order_updates (1).csv        |
                         +-----------------------------------+
                                           |
                                           v
                         +-----------------------------------+
                         |       Order Update Service        |
                         |   (Producer / CSV Streamer)       |
                         |  - O(1) Incremental CSV Reader    |
                         |  - Data Contract Validation       |
                         |  - 50 msgs/sec Config Rate Limit  |
                         +-----------------------------------+
                                           |
                                  HTTP POST /events
                                           |
                                           v
                         +-----------------------------------+
                         |    Position Maintaining Service   |
                         |   (Consumer / State Manager)      |
                         |  - In-Memory Net Position Store   |
                         |  - Zero-Alloc Idempotency Set     |
                         |  - Thread-Safe sync.RWMutex       |
                         |  - Server-Sent Events (SSE) Stream|
                         +-----------------------------------+
                                   |               |
                         HTTP GET /position     SSE /events/stream
                                   |               |
                                   v               v
                         +-----------------------------------+
                         |       Web Analytics Dashboard     |
                         |  - Real-Time Position Bar Chart   |
                         |  - Live Audit Log & Telemetry     |
                         |  - Interactive Stream Controller  |
                         +-----------------------------------+
```

### Inter-Service Communication Choice
- **Mechanism Selected**: Standard HTTP (`POST /events` and `GET /position`).
- **Rationale**: Standard HTTP requires zero external dependencies, is natively supported by Go's `net/http`, perfectly fits the "simple, correct" evaluation preference, and prevents unnecessary over-engineering.
- **Error Surfacing**: If the Position Maintaining Service is temporarily unavailable, the Order Update Service catches the network error, logs it, and continues.

---

## 3. Design of This Project

### Data Structures
- **Order Update Service**: Streaming iterator (`encoding/csv.Reader`). $O(1)$ memory usage.
- **Position Maintaining Service**:
  - Position Store: `map[string]int`. Keys are symbol strings, values are net integer positions.
  - Idempotency Store: `map[string]struct{}`. An empty struct (`struct{}`) in Go allocates zero bytes of memory, providing mathematically optimal memory allocation for sets.

### Concurrency Design
- Controlled via `sync.RWMutex`.
- `ProcessEvent` acquires `Lock()` (write lock).
- `GetPositions` acquires `RLock()` (read lock) and returns a **shallow copy** of the position map. Returning a shallow copy prevents fatal `concurrent map read and map write` panics when marshaling JSON over HTTP.

---

## 4. Security of This Project
- **Input Validation**: Defense in depth at producer and consumer layers.
  - Type strictness: `strconv.Atoi()` for integers.
  - Boundary checks: `quantity > 0`.
  - Enum strictness: `BUY` and `SELL` exact matching.
  - Blank string rejections.
- **Resilience Against DoS**: Memory footprint remains $O(1)$ regardless of CSV file size.

---

## 5. Review & Trade-offs
- **No Durable Delivery**: If the Position Service is offline, events are logged as network errors. Accepted to avoid heavy broker dependencies.
- **Idempotency Memory**: Seen `event_id` set lives in RAM. Ideal for finite CSV ingestion.

---

## 6. Development Phases
- **Phase 1**: Contract definition & Go module setup.
- **Phase 2**: Validator module & unit tests.
- **Phase 3**: Position Manager state & goroutine concurrency unit tests.
- **Phase 4**: Position Service HTTP API & SSE telemetry stream.
- **Phase 5**: Order Producer CSV streaming & rate limit throttler.
- **Phase 6**: Web Analytics Dashboard UI with Chart.js & CSV uploader.
- **Phase 7**: End-to-end verification & documentation.
