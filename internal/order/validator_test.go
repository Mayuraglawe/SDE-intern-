package order_test

import (
	"errors"
	"testing"

	"order-position-engine/internal/order"
	"order-position-engine/internal/shared"
)

func TestValidateRecord(t *testing.T) {
	tests := []struct {
		name        string
		record      []string
		wantEvent   *shared.OrderEvent
		wantErrIs   error
		expectError bool
	}{
		{
			name:   "Valid BUY record",
			record: []string{"evt-0001", "RELIANCE", "BUY", "90"},
			wantEvent: &shared.OrderEvent{
				EventID:         "evt-0001",
				Symbol:          "RELIANCE",
				TransactionType: shared.TxBuy,
				Quantity:        90,
			},
			expectError: false,
		},
		{
			name:   "Valid SELL record",
			record: []string{"evt-0002", "TCS", "SELL", "75"},
			wantEvent: &shared.OrderEvent{
				EventID:         "evt-0002",
				Symbol:          "TCS",
				TransactionType: shared.TxSell,
				Quantity:        75,
			},
			expectError: false,
		},
		{
			name:        "Blank event_id",
			record:      []string{"", "RELIANCE", "BUY", "90"},
			wantErrIs:   order.ErrBlankEventID,
			expectError: true,
		},
		{
			name:        "Blank symbol",
			record:      []string{"evt-0001", "", "BUY", "90"},
			wantErrIs:   order.ErrBlankSymbol,
			expectError: true,
		},
		{
			name:        "Lowercase transaction type (buy)",
			record:      []string{"evt-0001", "RELIANCE", "buy", "90"},
			wantErrIs:   order.ErrInvalidTxType,
			expectError: true,
		},
		{
			name:        "Invalid transaction type enum (HOLD)",
			record:      []string{"evt-0001", "RELIANCE", "HOLD", "90"},
			wantErrIs:   order.ErrInvalidTxType,
			expectError: true,
		},
		{
			name:        "Zero quantity",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "0"},
			wantErrIs:   order.ErrNonPositiveQuantity,
			expectError: true,
		},
		{
			name:        "Negative quantity",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "-100"},
			wantErrIs:   order.ErrNonPositiveQuantity,
			expectError: true,
		},
		{
			name:        "Non-integer quantity (float)",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "90.5"},
			wantErrIs:   order.ErrInvalidQuantityFormat,
			expectError: true,
		},
		{
			name:        "Non-integer quantity (string)",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "abc"},
			wantErrIs:   order.ErrInvalidQuantityFormat,
			expectError: true,
		},
		{
			name:        "Missing columns",
			record:      []string{"evt-0001", "RELIANCE"},
			wantErrIs:   order.ErrInvalidColumnCount,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvent, err := order.ValidateRecord(tt.record)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error containing %v, got nil", tt.wantErrIs)
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("expected error %v, got %v", tt.wantErrIs, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if *gotEvent != *tt.wantEvent {
					t.Errorf("got event %+v, want %+v", *gotEvent, *tt.wantEvent)
				}
			}
		})
	}
}

func TestValidateEvent(t *testing.T) {
	validEvent := &shared.OrderEvent{
		EventID:         "evt-100",
		Symbol:          "INFY",
		TransactionType: shared.TxBuy,
		Quantity:        50,
	}
	if err := order.ValidateEvent(validEvent); err != nil {
		t.Errorf("unexpected error for valid event: %v", err)
	}

	invalidEvent := &shared.OrderEvent{
		EventID:         "evt-101",
		Symbol:          "INFY",
		TransactionType: "INVALID",
		Quantity:        50,
	}
	if err := order.ValidateEvent(invalidEvent); err == nil {
		t.Errorf("expected error for invalid event transaction type, got nil")
	}
}
