package order

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"order-position-engine/internal/shared"
)

// EventSender handles posting validated OrderEvent structs to the Position Service over HTTP.
type EventSender struct {
	client    *http.Client
	targetURL string
}

// NewEventSender initializes a new sender with the target service URL and HTTP client.
func NewEventSender(targetURL string, client *http.Client) *EventSender {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &EventSender{
		client:    client,
		targetURL: targetURL,
	}
}

// Send serializes and posts an order event.
func (s *EventSender) Send(event *shared.OrderEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event JSON: %w", err)
	}

	resp, err := s.client.Post(s.targetURL, "application/json", bytes.NewBuffer(payload))
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
