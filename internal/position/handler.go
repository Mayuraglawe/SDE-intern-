package position

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"order-position-engine/internal/shared"
)

// SSEBroadcaster handles real-time Server-Sent Events streaming to web clients.
type SSEBroadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

// NewSSEBroadcaster creates a new broadcaster.
func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		clients: make(map[chan []byte]bool),
	}
}

func (b *SSEBroadcaster) AddClient(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[ch] = true
}

func (b *SSEBroadcaster) RemoveClient(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

func (b *SSEBroadcaster) Broadcast(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
			// Client buffer full; skip to prevent blocking
		}
	}
}

// HandlerDependencies wraps requirements for position HTTP endpoints.
type HandlerDependencies struct {
	Manager     *Manager
	Broadcaster *SSEBroadcaster
	WebDir      string
	AddAuditLog func(shared.ProcessResult)
	GetAuditLogs func() []shared.ProcessResult
}

// NewEventsHandler creates HTTP POST /events handler.
func NewEventsHandler(deps HandlerDependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var event shared.OrderEvent
		if err := json.Unmarshal(body, &event); err != nil {
			log.Printf("[POSITION CONSUMER REJECTED] Malformed JSON: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			res := shared.ProcessResult{
				Status: shared.StatusRejected,
				Reason: fmt.Sprintf("Malformed JSON: %v", err),
			}
			if deps.AddAuditLog != nil {
				deps.AddAuditLog(res)
			}
			json.NewEncoder(w).Encode(res)
			return
		}

		res, err := deps.Manager.ProcessEvent(event)
		if err != nil {
			log.Printf("[POSITION CONSUMER REJECTED] Event %s: %v", event.EventID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			if deps.AddAuditLog != nil {
				deps.AddAuditLog(res)
			}
			json.NewEncoder(w).Encode(res)
			return
		}

		log.Printf("[POSITION CONSUMER %s] Event %s | Symbol=%s Tx=%s Qty=%d -> NetPosition=%d",
			res.Status, event.EventID, event.Symbol, event.TransactionType, event.Quantity, res.NetPosition)

		if deps.AddAuditLog != nil {
			deps.AddAuditLog(res)
		}

		// Broadcast telemetry to connected SSE web clients
		if deps.Broadcaster != nil {
			if broadcastBytes, bErr := json.Marshal(res); bErr == nil {
				deps.Broadcaster.Broadcast(broadcastBytes)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(res)
	}
}

// NewPositionHandler creates GET /position handler.
func NewPositionHandler(deps HandlerDependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Serve HTML dashboard if navigated directly via browser Accept header
		acceptHeader := r.Header.Get("Accept")
		if r.URL.Query().Get("format") != "json" && strings.Contains(acceptHeader, "text/html") && !strings.Contains(acceptHeader, "application/json") {
			http.ServeFile(w, r, filepath.Join(deps.WebDir, "positions.html"))
			return
		}

		positions := deps.Manager.GetPositions()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(positions)
	}
}

// NewStreamHandler creates GET /events/stream SSE handler.
func NewStreamHandler(deps HandlerDependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		messageChan := make(chan []byte, 100)
		if deps.Broadcaster != nil {
			deps.Broadcaster.AddClient(messageChan)
			defer deps.Broadcaster.RemoveClient(messageChan)
		}

		// Send initial positions state & snapshot
		positions := deps.Manager.GetPositions()
		var logs []shared.ProcessResult
		if deps.GetAuditLogs != nil {
			logs = deps.GetAuditLogs()
		}

		initialState := struct {
			Type      string                 `json:"type"`
			Positions map[string]int         `json:"positions"`
			AuditLogs []shared.ProcessResult `json:"audit_logs"`
		}{
			Type:      "init",
			Positions: positions,
			AuditLogs: logs,
		}

		if initBytes, err := json.Marshal(initialState); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", string(initBytes))
			flusher.Flush()
		}

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-messageChan:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", string(msg))
				flusher.Flush()
			}
		}
	}
}
