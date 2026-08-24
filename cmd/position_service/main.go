package main

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"order-position-engine/internal/order"
	"order-position-engine/internal/position"
	"order-position-engine/internal/shared"
)

//go:embed seed_data.sql
var embeddedSeedSQL string

func loadSeedData(mgr *position.Manager, addLog func(shared.ProcessResult)) int {
	re := regexp.MustCompile(`VALUES\s*\(\s*'([^']+)'\s*,\s*'([^']+)'\s*,\s*'([^']+)'\s*,\s*(\d+)\s*\)`)
	matches := re.FindAllStringSubmatch(embeddedSeedSQL, -1)

	count := 0
	for _, match := range matches {
		if len(match) < 5 {
			continue
		}
		qty, err := strconv.Atoi(match[4])
		if err != nil {
			continue
		}
		event := shared.OrderEvent{
			EventID:         match[1],
			Symbol:          match[2],
			TransactionType: shared.TransactionType(match[3]),
			Quantity:        qty,
		}
		res, err := mgr.ProcessEvent(event)
		if err == nil {
			addLog(res)
			count++
		}
	}
	return count
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
	broadcaster := position.NewSSEBroadcaster()

	var auditMu sync.RWMutex
	var auditLogs []shared.ProcessResult
	var totalEventsCount, acceptedEventsCount, duplicateEventsCount, rejectedEventsCount int

	addAuditLog := func(res shared.ProcessResult) {
		auditMu.Lock()
		defer auditMu.Unlock()
		totalEventsCount++
		if res.Status == shared.StatusAccepted {
			acceptedEventsCount++
		} else if res.Status == shared.StatusDuplicate {
			duplicateEventsCount++
		} else if res.Status == shared.StatusRejected {
			rejectedEventsCount++
		}
		auditLogs = append([]shared.ProcessResult{res}, auditLogs...)
		if len(auditLogs) > 200 {
			auditLogs = auditLogs[:200]
		}
	}

	// Pre-load seed_data.sql events directly into memory on startup
	loadedSeedCount := loadSeedData(mgr, addAuditLog)
	log.Printf("[POSITION ENGINE SEED] Auto-loaded %d seed database order updates into memory on startup", loadedSeedCount)

	getAuditLogs := func() []shared.ProcessResult {
		auditMu.RLock()
		defer auditMu.RUnlock()
		cp := make([]shared.ProcessResult, len(auditLogs))
		copy(cp, auditLogs)
		return cp
	}

	clearAuditLogs := func() {
		auditMu.Lock()
		defer auditMu.Unlock()
		auditLogs = nil
		totalEventsCount = 0
		acceptedEventsCount = 0
		duplicateEventsCount = 0
		rejectedEventsCount = 0
	}

	deps := position.HandlerDependencies{
		Manager:      mgr,
		Broadcaster:  broadcaster,
		WebDir:       *webDir,
		AddAuditLog:  addAuditLog,
		GetAuditLogs: getAuditLogs,
	}

	mux := http.NewServeMux()

	// 1. POST /events - Ingest single event from Producer
	mux.HandleFunc("/events", position.NewEventsHandler(deps))

	// 2. GET /position & GET /api/position - Fetch current net position for all symbols seen
	posHandler := position.NewPositionHandler(deps)
	mux.HandleFunc("/position", posHandler)
	mux.HandleFunc("/api/position", posHandler)

	// 3. GET /events/stream - Server-Sent Events stream for live dashboard
	mux.HandleFunc("/events/stream", func(w http.ResponseWriter, r *http.Request) {
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
		broadcaster.AddClient(messageChan)
		defer broadcaster.RemoveClient(messageChan)

		// Send initial positions state and recent audit logs
		positions := mgr.GetPositions()
		auditMu.RLock()
		tEvt, aEvt, dEvt, rEvt := totalEventsCount, acceptedEventsCount, duplicateEventsCount, rejectedEventsCount
		auditMu.RUnlock()

		initData, _ := json.Marshal(map[string]interface{}{
			"type":             "INIT",
			"positions":        positions,
			"recent_events":    getAuditLogs(),
			"total_events":     tEvt,
			"accepted_events":  aEvt,
			"duplicate_events": dEvt,
			"rejected_events":  rEvt,
		})
		fmt.Fprintf(w, "data: %s\n\n", initData)
		flusher.Flush()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
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

		results := make([]shared.ProcessResult, 0)
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
				if len(record) > 0 && strings.EqualFold(record[0], "event_id") {
					continue
				}
			}

			event, valErr := order.ValidateRecord(record)
			if valErr != nil {
				res := shared.ProcessResult{
					Status: shared.StatusRejected,
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
