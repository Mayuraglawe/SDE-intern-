package producer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"order-position-engine/internal/models"
	"order-position-engine/internal/validator"
)

// StreamerConfig holds runtime options for streaming order updates.
type StreamerConfig struct {
	CSVFilePath  string
	TargetURL    string
	RateLimitMPS int // Messages per second (e.g., 50)
	HTTPClient   *http.Client
}

// StreamStats tracks processing metrics during streaming.
type StreamStats struct {
	TotalRows    int
	AcceptedRows int
	SkippedRows  int
	SentEvents   int
	HTTPFailures int
	StartTime    time.Time
	EndTime      time.Time
}

// Streamer reads CSV incrementally and posts valid events to Position Service.
type Streamer struct {
	cfg StreamerConfig
}

// NewStreamer creates a new streamer instance.
func NewStreamer(cfg StreamerConfig) *Streamer {
	if cfg.RateLimitMPS <= 0 {
		cfg.RateLimitMPS = 50
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Streamer{cfg: cfg}
}

// Stream reads the CSV file line by line and dispatches events.
func (s *Streamer) Stream() (*StreamStats, error) {
	file, err := os.Open(s.cfg.CSVFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file %s: %w", s.cfg.CSVFilePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable columns for robust validation handling

	stats := &StreamStats{StartTime: time.Now()}
	interval := time.Second / time.Duration(s.cfg.RateLimitMPS)

	log.Printf("[PRODUCER] Starting order stream from %s -> %s (Rate limit: %d mps / interval %v)",
		s.cfg.CSVFilePath, s.cfg.TargetURL, s.cfg.RateLimitMPS, interval)

	isFirstRow := true
	for {
		startRowTime := time.Now()
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Printf("[PRODUCER WARN] CSV reader error at row %d: %v. Skipping.", stats.TotalRows+1, err)
			stats.SkippedRows++
			stats.TotalRows++
			continue
		}

		stats.TotalRows++

		// Skip header row if present
		if isFirstRow {
			isFirstRow = false
			if len(record) > 0 && strings.EqualFold(record[0], "event_id") {
				log.Printf("[PRODUCER INFO] Skipped CSV header: %v", record)
				continue
			}
		}

		// Validate record strictly
		event, valErr := validator.ValidateRecord(record)
		if valErr != nil {
			log.Printf("[PRODUCER REJECTED] Row %d rejected: %v | Raw: %v", stats.TotalRows, valErr, record)
			stats.SkippedRows++
			continue
		}

		stats.AcceptedRows++

		// Post valid event to Position Service
		if sendErr := s.sendEvent(event); sendErr != nil {
			log.Printf("[PRODUCER ERROR] Failed to send %s to %s: %v", event.EventID, s.cfg.TargetURL, sendErr)
			stats.HTTPFailures++
		} else {
			stats.SentEvents++
			log.Printf("[PRODUCER SENT] Row %d | Sent %s | Symbol=%s Tx=%s Qty=%d",
				stats.TotalRows, event.EventID, event.Symbol, event.TransactionType, event.Quantity)
		}

		// Enforce rate throttling
		elapsed := time.Since(startRowTime)
		if elapsed < interval {
			time.Sleep(interval - elapsed)
		}
	}

	stats.EndTime = time.Now()
	duration := stats.EndTime.Sub(stats.StartTime)
	log.Printf("[PRODUCER COMPLETE] Finished processing %d rows in %v (Accepted: %d, Skipped: %d, Sent: %d, HTTP Failures: %d)",
		stats.TotalRows, duration, stats.AcceptedRows, stats.SkippedRows, stats.SentEvents, stats.HTTPFailures)

	return stats, nil
}

func (s *Streamer) sendEvent(event *models.OrderEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event JSON: %w", err)
	}

	resp, err := s.cfg.HTTPClient.Post(s.cfg.TargetURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("network connection error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
