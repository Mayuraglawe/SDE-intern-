# System Architecture

This document details the High-Level Design (HLD), microservice communication patterns, data flow pipelines, and component relationships of the Go Order Position Engine.

---

## 1. System Topology Diagram

```mermaid
flowchart TD
    subgraph Data Source
        CSV[order_updates.csv]
    end

    subgraph Order Update Service (Producer Process)
        Reader[Incremental CSV Reader O(1) RAM]
        Val[Data Contract Validator]
        Throttle[Rate Limit Throttler - 50 MPS]
        Sender[HTTP POST Dispatcher]

        CSV --> Reader
        Reader --> Val
        Val -->|Valid Event| Throttle
        Throttle --> Sender
    end

    subgraph Position Maintaining Service (Consumer Process)
        Handler[POST /events Endpoint]
        Manager[Position & Idempotency Engine]
        Store[(In-Memory State: map[string]int)]
        Dedup[(Idempotency Set: map[string]struct{})]
        GetAPI[GET /position Endpoint]
        SSE[Server-Sent Events Broadcast]

        Sender -->|HTTP POST /events| Handler
        Handler --> Manager
        Manager <--> Store
        Manager <--> Dedup
        Manager --> SSE
    end

    subgraph Web Analytics Dashboard
        UI[Browser UI & Chart.js]
        GetAPI -->|Fetch Positions| UI
        SSE -->|Live Event Stream| UI
    end
```

---

## 2. Component Microservice Breakdown

### A. Order Update Service (CSV Producer)
- **Entry Point**: [`cmd/order_service/main.go`](file:///d:/SDE%20Intern/cmd/order_service/main.go)
- **Domain Package**: [`internal/order`](file:///d:/SDE%20Intern/internal/order)
- **Key Duties**:
  1. Opens CSV source file and iterates line-by-line via `csv.Reader`.
  2. Applies strict data contract validation rules.
  3. Enforces 50 events/sec throttling via time-delta sleep calculations.
  4. Marshals validated `OrderEvent` structs to JSON and posts them to `http://localhost:8080/events`.

### B. Position Maintaining Service (Consumer & State Engine)
- **Entry Point**: [`cmd/position_service/main.go`](file:///d:/SDE%20Intern/cmd/position_service/main.go)
- **Domain Package**: [`internal/position`](file:///d:/SDE%20Intern/internal/position)
- **Key Duties**:
  1. Receives incoming HTTP `POST /events` requests.
  2. Validates idempotency against seen `event_id` keys.
  3. Updates net positions (`BUY` adds, `SELL` subtracts).
  4. Serves `GET /position` API endpoint returning symbol net balances.
  5. Pushes telemetry to web clients over Server-Sent Events (SSE) `GET /events/stream`.

---

## 3. Inter-Service Communication Choices

- **Protocol**: Synchronous HTTP/1.1 REST over TCP (`POST /events`).
- **Data Format**: `application/json` payload representing `shared.OrderEvent`:
  ```json
  {
    "event_id": "evt-0001",
    "symbol": "RELIANCE",
    "transaction_type": "BUY",
    "quantity": 90
  }
  ```
- **Rationale**: Standard HTTP requires zero external broker dependencies (e.g. Kafka/Redis), provides immediate status code feedback (`202 Accepted` / `400 Bad Request`), and ensures simple, reproducible execution.
- **Fault Resilience**: If the Position Service is offline, the Order Service logs connection errors to console without crashing, allowing stream completion metrics to track `HTTPFailures`.
