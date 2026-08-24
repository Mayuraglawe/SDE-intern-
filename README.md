# Go Order Position Engine & Live Analytics Platform

> **Indo Thai SDE Intern Take-Home Assessment Implementation**  
> A high-performance, resilient microservice architecture built with Go 1.26 that streams trading order updates incrementally, validates input data strictly, maintains real-time in-memory symbol positions with zero-allocation idempotency, and presents a visual Web Analytics Dashboard.

---

## 🏛️ Architecture & System Overview

The system consists of two independent Go microservice processes communicating over HTTP, alongside an integrated real-time Web Analytics Platform:

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

### Component Breakdown & Design Choices
1. **Architecture Scope Choice**:
   - Uses a straightforward **two-service Client-Server pattern** rather than complex distributed microservice orchestration (e.g. Kubernetes, service mesh, API gateways, or message brokers) because the requirements explicitly mark distributed deployment and persistent databases as non-goals. This achieves strict process separation while avoiding unnecessary operational complexity.
2. **Order Update Service (Producer)**:
   - **Incremental Ingestion**: Uses Go's standard `encoding/csv` reader row-by-row. Maintains $O(1)$ memory consumption regardless of file size.
   - **Validation Layer**: Rejects blank fields, invalid transaction types, floats, and zero/negative quantities.
   - **Rate Throttler**: Enforces a strict, configurable rate limit (default: 50 events/sec) using time delta calculations.
3. **Position Maintaining Service (Consumer)**:
   - **Position Store**: `map[string]int` tracking net positions (`BUY` adds, `SELL` subtracts). Retains symbol entries even when net position is zero or negative.
   - **Idempotency Set**: `map[string]struct{}` storing seen `event_id` keys. In Go, `struct{}` allocates 0 bytes, minimizing memory growth. First valid `event_id` wins; duplicates are ignored.
   - **Thread Safety & Concurrent Read/Write Protection**: State operations use `sync.RWMutex`. Read operations (`GET /position`) return a **shallow copy** of the position map to prevent concurrent map mutation panics.
   - **Real-Time Telemetry Stream**: Exposes Server-Sent Events (SSE) `/events/stream` to push updates directly to web clients.
4. **Web Analytics Platform ("Proper Platform")**:
   - Built with Vanilla HTML5, CSS3 (Glassmorphism Dark Mode UI), JavaScript, and Chart.js.
   - Features live symbol position bar charts, telemetry cards, audit log search filter, manual REST API query tool, and direct drag-and-drop CSV batch uploader.

---

## 🚀 Step-by-Step Setup & Run Instructions

### Prerequisites
- **Go**: Version `1.20` or higher installed. (Tested on Go `1.26.0`)
- **Web Browser**: Chrome, Edge, Firefox, or Safari for the Web Dashboard.

### Step 1: Clone & Navigate to Project Directory
```powershell
cd "d:\SDE Intern"
```

### Step 2: Start Position Maintaining Service
Open terminal 1 and run:
```powershell
go run cmd/position_service/main.go --port=8080
```
*The Position Service will launch on `http://localhost:8080` and serve both REST APIs and the Web Analytics Dashboard.*

### Step 3: Launch Order Update Service (CSV Producer)
Open terminal 2 and run:
```powershell
go run cmd/order_service/main.go --csv-file="order_updates (1).csv" --target-url="http://localhost:8080/events" --rate-limit-mps=50
```
*The Order Service will begin streaming 1,002 rows from `order_updates (1).csv` at 50 events/sec to the Position Service.*

### Step 4: Access Web Platform Dashboard
Open your browser and navigate to:
```
http://localhost:8080
```

---

## 🧪 Running Automated Unit Tests

Run all unit tests across all packages:
```powershell
go test -v ./...
```

### Coverage Details:
- **`internal/validator`**: Tests BUY/SELL validation, blank event IDs, blank symbols, invalid transaction types (`buy`, `HOLD`), floats, negative quantities, zero quantities, and malformed CSV rows.
- **`internal/position`**: Tests BUY/SELL position calculations, negative net positions, zero balance retention, duplicate `event_id` suppression (first wins), and concurrent goroutine access safety.

---

## ⚙️ Configuration Options (CLI Flags)

### Position Maintaining Service (`cmd/position_service/main.go`)
| Flag | Default | Description |
| :--- | :--- | :--- |
| `--port` | `8080` | Port number to bind HTTP server |
| `--web-dir` | `./web` | Directory path containing web frontend assets |

### Order Update Service (`cmd/order_service/main.go`)
| Flag | Default | Description |
| :--- | :--- | :--- |
| `--csv-file` | `order_updates (1).csv` | Path to CSV order updates file |
| `--target-url` | `http://localhost:8080/events` | Target HTTP POST URL of Position Service |
| `--rate-limit-mps` | `50` | Maximum order updates to emit per second |

---

## 📡 REST API Examples & Responses

### 1. Ingest Single Order Update Event
**Request**: `POST /events`
```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{
    "event_id": "evt-0001",
    "symbol": "RELIANCE",
    "transaction_type": "BUY",
    "quantity": 90
  }'
```

**Response** (`202 Accepted`):
```json
{
  "status": "ACCEPTED",
  "event_id": "evt-0001",
  "symbol": "RELIANCE",
  "net_position": 90,
  "timestamp": "2026-08-24T12:45:00.123456Z"
}
```

### 2. Fetch Current Net Position
**Request**: `GET /position`
```bash
curl http://localhost:8080/position
```

**Response** (`200 OK`):
```json
{
  "ADANIENT": 100,
  "ASIANPAINT": -25,
  "AXISBANK": 200,
  "BAJFINANCE": 50,
  "BHARTIARTL": -75,
  "HDFCBANK": 300,
  "HINDUNILVR": -275,
  "ICICIBANK": -225,
  "INFY": 150,
  "ITC": 350,
  "KOTAKBANK": -125,
  "LT": -425,
  "MARUTI": -475,
  "NTPC": -325,
  "RELIANCE": 450,
  "SBIN": 500,
  "SUNPHARMA": 400,
  "TATAMOTORS": 250,
  "TATASTEEL": -175,
  "TCS": -375
}
```

### 3. Reset Engine State
**Request**: `POST /reset`
```bash
curl -X POST http://localhost:8080/reset
```

---

## ⚖️ Known Limitations & Trade-offs

1. **No Durable Delivery / Message Broker**:
   - If the Position Service process is killed while the Order Service is streaming, dropped events are logged to terminal console. Standard HTTP was chosen to eliminate external broker dependencies (e.g. Kafka/Redis) for zero-setup execution.
2. **In-Memory State**:
   - Position state and seen `event_id` sets are stored entirely in RAM and reset upon process restart, as specified in the assessment non-goals.
3. **Idempotency Memory Growth**:
   - Idempotency set keys grow linearly with unique `event_id` count. Utilizing Go's `map[string]struct{}` ensures zero byte allocation for values, but for infinite streams, a TTL cache (e.g., Redis or RingBuffer) would be required.

---

## 📁 Repository Directory Structure

```text
.
├── cmd/
│   ├── order_service/
│   │   └── main.go          # CSV Producer process entrypoint
│   └── position_service/
│       ├── main.go          # State Consumer & Web API server
│       └── seed_data.sql    # Startup seed data
├── internal/
│   ├── order/
│   │   ├── config.go        # Configuration & stream metrics stats
│   │   ├── csv_reader.go    # Line-by-line CSV reader & streamer
│   │   ├── validator.go     # Data contract validator (event_id, symbol, tx_type, qty)
│   │   ├── validator_test.go# Unit tests for validation edge cases
│   │   ├── throttler.go     # 50 events/sec rate limiter
│   │   └── sender.go        # HTTP POST event dispatcher
│   ├── position/
│   │   ├── manager.go       # Concurrency-safe state & RLock idempotency manager
│   │   ├── manager_test.go  # Unit tests for position calculation & concurrency
│   │   └── handler.go       # HTTP endpoint handlers (/events, /position, /events/stream)
│   └── shared/
│       └── models.go        # OrderEvent, ProcessResult, & TransactionType enums
├── web/
│   ├── index.html           # Web Platform UI Layout
│   ├── positions.html       # Position View HTML Layout
│   ├── style.css            # Modern Glassmorphism CSS styles
│   └── app.js               # Real-time SSE listener & Chart.js logic
├── docs/
│   └── System_Documentation.md # System architecture documentation
├── go.mod                   # Go module definition
├── order_updates (1).csv    # Assessment dataset (1002 rows)
└── README.md                # Project documentation & run guide
```
