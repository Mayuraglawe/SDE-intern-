package shared

import "fmt"

// TransactionType represents valid transaction actions (BUY or SELL).
type TransactionType string

const (
	TxBuy  TransactionType = "BUY"
	TxSell TransactionType = "SELL"
)

// OrderEvent represents a single ingested order update event.
type OrderEvent struct {
	EventID         string          `json:"event_id"`
	Symbol          string          `json:"symbol"`
	TransactionType TransactionType `json:"transaction_type"`
	Quantity        int             `json:"quantity"`
}

// EventProcessStatus represents the outcome of processing an event in Position Service.
type EventProcessStatus string

const (
	StatusAccepted  EventProcessStatus = "ACCEPTED"
	StatusDuplicate EventProcessStatus = "DUPLICATE"
	StatusRejected  EventProcessStatus = "REJECTED"
)

// ProcessResult detail for event responses / broadcast telemetry.
type ProcessResult struct {
	Status      EventProcessStatus `json:"status"`
	EventID     string             `json:"event_id"`
	Symbol      string             `json:"symbol"`
	NetPosition int                `json:"net_position,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	Timestamp   string             `json:"timestamp,omitempty"`
}

func (e OrderEvent) String() string {
	return fmt.Sprintf("Event[ID=%s, Symbol=%s, Tx=%s, Qty=%d]", e.EventID, e.Symbol, e.TransactionType, e.Quantity)
}
