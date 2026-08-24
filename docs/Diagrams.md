# Architectural Diagrams & Visual Workflows

This document consolidates all Mermaid visual diagrams illustrating the High-Level Design (HLD), Low-Level Design (LLD), sequence flows, validation decision trees, and state lifecycles of the Go Order Position Engine.

---

## 1. High-Level System Topology Diagram (HLD)

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

        CSV -->|Line by Line| Reader
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

## 2. Inter-Service Communication Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    participant CSV as CSV Reader
    participant Val as Validator
    participant Thr as Throttler (50 MPS)
    participant Sender as HTTP Sender
    participant Consumer as Position Service
    participant Web as Web Dashboard

    CSV->>Val: Raw CSV Row []string
    alt Record Invalid
        Val-->>CSV: Rejection Error (Skip Row)
    else Record Valid
        Val->>Thr: Valid OrderEvent
        Thr->>Thr: Sleep (interval - elapsed)
        Thr->>Sender: Rate-limited OrderEvent
        Sender->>Consumer: HTTP POST /events (JSON Payload)
        activate Consumer
        Consumer->>Consumer: ProcessEvent (RWMutex Lock)
        Consumer-->>Sender: HTTP 202 Accepted (ProcessResult JSON)
        deactivate Consumer
        Consumer->>Web: SSE Broadcast (/events/stream)
    end
```

---

## 3. Order Data Validation Decision Tree

```mermaid
flowchart TD
    Start([Raw CSV Row]) --> Q1{Columns >= 4?}
    Q1 -- No --> Err1[Reject: ErrInvalidColumnCount]
    Q1 -- Yes --> Q2{event_id Non-Empty?}
    Q2 -- No --> Err2[Reject: ErrBlankEventID]
    Q2 -- Yes --> Q3{symbol Non-Empty?}
    Q3 -- No --> Err3[Reject: ErrBlankSymbol]
    Q3 -- Yes --> Q4{tx_type == BUY or SELL?}
    Q4 -- No --> Err4[Reject: ErrInvalidTxType]
    Q4 -- Yes --> Q5{quantity > 0 Integer?}
    Q5 -- No --> Err5[Reject: ErrNonPositiveQuantity]
    Q5 -- Yes --> Pass([Valid OrderEvent -> Dispatch])
```

---

## 4. Position State Engine & Idempotency Lifecycle

```mermaid
flowchart TD
    In([OrderEvent Received]) --> Lock[Acquire RWMutex Write Lock]
    Lock --> CheckDup{event_id in seenEvents?}
    CheckDup -- Yes --> DupRes[Return StatusDuplicate | NetPosition unchanged]
    CheckDup -- No --> RecordID[Add event_id to seenEvents map]
    RecordID --> CheckTx{TransactionType?}
    CheckTx -- BUY --> Add[positions[symbol] += quantity]
    CheckTx -- SELL --> Sub[positions[symbol] -= quantity]
    Add --> AccRes[Return StatusAccepted | Updated NetPosition]
    Sub --> AccRes
    DupRes --> Unlock[Release RWMutex Lock]
    AccRes --> Unlock
```

---

## 5. Package Dependency Hierarchy Diagram

```mermaid
graph TD
    cmdOrder[cmd/order_service/main.go] --> internalOrder[internal/order]
    cmdPos[cmd/position_service/main.go] --> internalPos[internal/position]
    cmdPos --> internalOrder
    cmdPos --> internalShared[internal/shared]
    internalOrder --> internalShared
    internalPos --> internalShared
```

---

## 6. Implementation Phases Roadmap Timeline

```mermaid
timeline
    title Project Implementation Roadmap
    Phase 1 : Data Models & Contracts : Enums & Shared Structs
    Phase 2 : Data Validation Engine : Strict Rules & Unit Tests
    Phase 3 : Position Manager : State Engine & Goroutine Mutex Tests
    Phase 4 : Position Maintaining Service : HTTP API Endpoints & SSE Stream
    Phase 5 : Order Update Service : Streaming CSV Reader & 50 MPS Throttler
    Phase 6 : Web Analytics Platform : Real-time Glassmorphism UI & Chart.js
    Phase 7 : Verification & Audit : Full Test Suite & Documentation
```
