package order

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Streamer reads CSV incrementally and posts valid events to Position Service.
type Streamer struct {
	cfg       Config
	throttler *Throttler
	sender    *EventSender
}

// NewStreamer creates a new streamer instance.
func NewStreamer(cfg Config) *Streamer {
	if cfg.RateLimitMPS <= 0 {
		cfg.RateLimitMPS = 50
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Streamer{
		cfg:       cfg,
		throttler: NewThrottler(cfg.RateLimitMPS),
		sender:    NewEventSender(cfg.TargetURL, cfg.HTTPClient),
	}
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

	log.Printf("[ORDER SERVICE] Starting order stream from %s -> %s (Rate limit: %d mps)",
		s.cfg.CSVFilePath, s.cfg.TargetURL, s.cfg.RateLimitMPS)

	isFirstRow := true
	for {
		startRowTime := time.Now()
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Printf("[ORDER PRODUCER WARN] CSV reader error at row %d: %v. Skipping.", stats.TotalRows+1, err)
			stats.SkippedRows++
			stats.TotalRows++
			continue
		}

		stats.TotalRows++

		// Skip header row if present
		if isFirstRow {
			isFirstRow = false
			if len(record) > 0 && strings.EqualFold(record[0], "event_id") {
				log.Printf("[ORDER PRODUCER INFO] Skipped CSV header: %v", record)
				continue
			}
		}

		// Validate record strictly
		event, valErr := ValidateRecord(record)
		if valErr != nil {
			log.Printf("[ORDER PRODUCER REJECTED] Row %d rejected: %v | Raw: %v", stats.TotalRows, valErr, record)
			stats.SkippedRows++
			continue
		}

		stats.AcceptedRows++

		// Post valid event to Position Service
		if sendErr := s.sender.Send(event); sendErr != nil {
			log.Printf("[ORDER PRODUCER ERROR] Failed to send %s to %s: %v", event.EventID, s.cfg.TargetURL, sendErr)
			stats.HTTPFailures++
		} else {
			stats.SentEvents++
			log.Printf("[ORDER PRODUCER SENT] Row %d | Sent %s | Symbol=%s Tx=%s Qty=%d",
				stats.TotalRows, event.EventID, event.Symbol, event.TransactionType, event.Quantity)
		}

		// Enforce rate throttling
		elapsed := time.Since(startRowTime)
		s.throttler.Enforce(elapsed)
	}

	stats.EndTime = time.Now()
	duration := stats.EndTime.Sub(stats.StartTime)
	log.Printf("[ORDER PRODUCER COMPLETE] Finished processing %d rows in %v (Accepted: %d, Skipped: %d, Sent: %d, HTTP Failures: %d)",
		stats.TotalRows, duration, stats.AcceptedRows, stats.SkippedRows, stats.SentEvents, stats.HTTPFailures)

	return stats, nil
}
