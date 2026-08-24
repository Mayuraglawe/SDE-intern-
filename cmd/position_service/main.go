package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"order-position-engine/internal/models"
	"order-position-engine/internal/position"
	"order-position-engine/internal/validator"
)

// SSEBroadcaster handles real-time Server-Sent Events streaming to web clients.
type SSEBroadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

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

func main() {
	defaultPort := 8080
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			defaultPort = p
		}
	}
	port := flag.Int("port", defaultPort, "Port for Position Maintaining Service")
	webDir := flag.String("web-dir", "./web", "Directory containing web frontend files")
	flag.Parse()

	mgr := position.NewManager()
	broadcaster := NewSSEBroadcaster()

	var auditMu sync.RWMutex
	var auditLogs []models.ProcessResult

	addAuditLog := func(res models.ProcessResult) {
		auditMu.Lock()
		defer auditMu.Unlock()
		auditLogs = append([]models.ProcessResult{res}, auditLogs...)
		if len(auditLogs) > 200 {
			auditLogs = auditLogs[:200]
		}
	}

	getAuditLogs := func() []models.ProcessResult {
		auditMu.RLock()
		defer auditMu.RUnlock()
		cp := make([]models.ProcessResult, len(auditLogs))
		copy(cp, auditLogs)
		return cp
	}

	clearAuditLogs := func() {
		auditMu.Lock()
		defer auditMu.Unlock()
		auditLogs = nil
	}

	mux := http.NewServeMux()

	// 1. POST /events - Ingest single event from Producer
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
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

		var event models.OrderEvent
		if err := json.Unmarshal(body, &event); err != nil {
			log.Printf("[POSITION CONSUMER REJECTED] Malformed JSON: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			res := models.ProcessResult{
				Status: models.StatusRejected,
				Reason: fmt.Sprintf("Malformed JSON: %v", err),
			}
			addAuditLog(res)
			json.NewEncoder(w).Encode(res)
			return
		}

		res, err := mgr.ProcessEvent(event)
		if err != nil {
			log.Printf("[POSITION CONSUMER REJECTED] Event %s: %v", event.EventID, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			addAuditLog(res)
			json.NewEncoder(w).Encode(res)
			return
		}

		log.Printf("[POSITION CONSUMER %s] Event %s | Symbol=%s Tx=%s Qty=%d -> NetPosition=%d",
			res.Status, event.EventID, event.Symbol, event.TransactionType, event.Quantity, res.NetPosition)

		addAuditLog(res)

		// Broadcast telemetry to connected SSE web clients
		if broadcastBytes, bErr := json.Marshal(res); bErr == nil {
			broadcaster.Broadcast(broadcastBytes)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(res)
	})

	// 2. GET /position & GET /api/position - Fetch current net position for all symbols seen
	handlePositionJSON := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// If opened directly in browser navigation bar, serve the formatted positions.html page!
		acceptHeader := r.Header.Get("Accept")
		if r.URL.Query().Get("format") != "json" && strings.Contains(acceptHeader, "text/html") && !strings.Contains(acceptHeader, "application/json") {
			http.ServeFile(w, r, filepath.Join(*webDir, "positions.html"))
			return
		}

		positions := mgr.GetPositions()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(positions)
	}

	mux.HandleFunc("/position", handlePositionJSON)
	mux.HandleFunc("/api/position", handlePositionJSON)

	// 3. GET /events/stream - Server-Sent Events stream for live dashboard
	mux.HandleFunc("/events/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		messageChan := make(chan []byte, 100)
		broadcaster.AddClient(messageChan)
		defer broadcaster.RemoveClient(messageChan)

		// Send initial positions state and recent audit logs
		positions := mgr.GetPositions()
		initData, _ := json.Marshal(map[string]interface{}{
			"type":          "INIT",
			"positions":     positions,
			"recent_events": getAuditLogs(),
		})
		fmt.Fprintf(w, "data: %s\n\n", initData)
		flusher.Flush()

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
	})

	// 4. POST /events/bulk - Upload & ingest CSV directly via Web Dashboard
	mux.HandleFunc("/events/bulk", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing 'file' form field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		reader := csv.NewReader(file)
		reader.FieldsPerRecord = -1

		results := make([]models.ProcessResult, 0)
		isFirst := true

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}

			if isFirst {
				isFirst = false
				if len(record) > 0 && record[0] == "event_id" {
					continue
				}
			}

			event, valErr := validator.ValidateRecord(record)
			if valErr != nil {
				res := models.ProcessResult{
					Status: models.StatusRejected,
					Reason: valErr.Error(),
				}
				results = append(results, res)
				addAuditLog(res)
				continue
			}

			res, _ := mgr.ProcessEvent(*event)
			results = append(results, res)
			addAuditLog(res)

			if bBytes, bErr := json.Marshal(res); bErr == nil {
				broadcaster.Broadcast(bBytes)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"processed": len(results),
			"positions": mgr.GetPositions(),
		})
	})

	// 5. POST /reset - Clear position manager state
	mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		mgr.Reset()
		clearAuditLogs()
		if broadcastBytes, bErr := json.Marshal(map[string]interface{}{
			"type": "RESET",
		}); bErr == nil {
			broadcaster.Broadcast(broadcastBytes)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "State reset successfully"})
	})

	// 6. Serve Web Frontend
	absWebDir, _ := filepath.Abs(*webDir)
	if _, err := os.Stat(absWebDir); err == nil {
		fs := http.FileServer(http.Dir(absWebDir))
		mux.Handle("/", fs)
		log.Printf("[POSITION SERVICE] Serving Web Dashboard from %s", absWebDir)
	} else {
		log.Printf("[POSITION SERVICE WARN] Web directory %s not found. API endpoints active.", absWebDir)
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("==========================================================")
	log.Printf(" Position Maintaining Service Running at http://localhost%s", addr)
	log.Printf(" - GET /position       (Fetch current net symbol positions)")
	log.Printf(" - POST /events        (Ingest order update event)")
	log.Printf(" - GET /events/stream  (Real-time SSE event telemetry stream)")
	log.Printf(" - GET /               (Web Analytics Platform)")
	log.Printf("==========================================================")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Position Service failed to start: %v", err)
	}
}
