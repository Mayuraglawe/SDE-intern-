# Business & Functional Rules

This document outlines the strict business rules, data validation contracts, idempotency rules, and non-negotiable architectural constraints governing the Go Order Position Engine.

---

## 1. Functional Rules

1. **Process Separation**: The **Order Update Service** (Producer) and the **Position Maintaining Service** (Consumer) must execute as completely separate OS processes communicating exclusively over HTTP network protocols.
2. **Incremental CSV Streaming**: The Order Update Service must read CSV datasets line-by-line using Go's `encoding/csv` reader. Loading the entire CSV file into memory ($O(N)$ allocation) is strictly prohibited. Memory consumption must remain constant ($O(1)$).
3. **Sequential Order Preservation**: Valid order updates must be dispatched to the Position Maintaining Service in the exact sequence they appear within the source CSV dataset.
4. **Rate Throttling**: The Order Update Service must emit events at a rate not exceeding 50 events per second (configurable via the `-rate-limit-mps` CLI flag).
5. **Concurrent Safety**: Incoming position updates (`POST /events`) and position read queries (`GET /position`) must execute concurrently without data race conditions, memory corruption, or map read/write panics.
6. **Zero-Allocation Idempotency**: The first valid event received for a given `event_id` is accepted (*first-wins strategy*). Subsequent events presenting an already-processed `event_id` must be suppressed without altering symbol net positions.
7. **Graceful Fault Tolerance**: Malformed CSV rows or invalid payloads must never crash either service. Invalid events must be rejected with a clear error reason, logged, and skipped.

---

## 2. Data Contract Rules

Every incoming order update must strictly satisfy all four validation criteria:

| Field Name | Type | Validation Rule | Error Trigger |
| :--- | :--- | :--- | :--- |
| **`event_id`** | String | Must be a non-empty string after trimming whitespace. | `ErrBlankEventID` |
| **`symbol`** | String | Must be a non-empty string. Case and characters preserved as provided. | `ErrBlankSymbol` |
| **`transaction_type`** | Enum | Must be exactly `"BUY"` or `"SELL"` (case-sensitive). Lowercase or arbitrary strings rejected. | `ErrInvalidTxType` |
| **`quantity`** | Integer | Must be a strictly positive integer ($> 0$). Zero, negative values, and floating-point numbers rejected. | `ErrNonPositiveQuantity` / `ErrInvalidQuantityFormat` |

---

## 3. Out-of-Scope Boundaries (Constraints)

To prevent over-engineering and adhere strictly to assessment guidelines:
- **No Disk Database**: Persistent storage engines (SQL/NoSQL) are not required for core position processing. State lives in memory.
- **No External Message Brokers**: RabbitMQ, Apache Kafka, and Redis dependencies are excluded to maintain zero-setup execution.
- **No TLS / Authentication**: Security layers such as JWT, OAuth, or HTTPS are omitted for local microservice processing.
- **No Distributed Orchestration**: Kubernetes or complex service meshes are excluded.
