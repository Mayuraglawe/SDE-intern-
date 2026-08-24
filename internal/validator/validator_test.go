package validator_test

import (
	"errors"
	"testing"

	"order-position-engine/internal/models"
	"order-position-engine/internal/validator"
)

func TestValidateRecord(t *testing.T) {
	tests := []struct {
		name        string
		record      []string
		wantEvent   *models.OrderEvent
		wantErrIs   error
		expectError bool
	}{
		{
			name:   "Valid BUY record",
			record: []string{"evt-0001", "RELIANCE", "BUY", "90"},
			wantEvent: &models.OrderEvent{
				EventID:         "evt-0001",
				Symbol:          "RELIANCE",
				TransactionType: models.TxBuy,
				Quantity:        90,
			},
			expectError: false,
		},
		{
			name:   "Valid SELL record",
			record: []string{"evt-0002", "TCS", "SELL", "75"},
			wantEvent: &models.OrderEvent{
				EventID:         "evt-0002",
				Symbol:          "TCS",
				TransactionType: models.TxSell,
				Quantity:        75,
			},
			expectError: false,
		},
		{
			name:        "Blank event_id",
			record:      []string{"", "RELIANCE", "BUY", "90"},
			wantErrIs:   validator.ErrBlankEventID,
			expectError: true,
		},
		{
			name:        "Blank symbol",
			record:      []string{"evt-0001", "", "BUY", "90"},
			wantErrIs:   validator.ErrBlankSymbol,
			expectError: true,
		},
		{
			name:        "Lowercase transaction type (buy)",
			record:      []string{"evt-0001", "RELIANCE", "buy", "90"},
			wantErrIs:   validator.ErrInvalidTxType,
			expectError: true,
		},
		{
			name:        "Invalid transaction type enum (HOLD)",
			record:      []string{"evt-0001", "RELIANCE", "HOLD", "90"},
			wantErrIs:   validator.ErrInvalidTxType,
			expectError: true,
		},
		{
			name:        "Zero quantity",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "0"},
			wantErrIs:   validator.ErrNonPositiveQuantity,
			expectError: true,
		},
		{
			name:        "Negative quantity",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "-100"},
			wantErrIs:   validator.ErrNonPositiveQuantity,
			expectError: true,
		},
		{
			name:        "Non-integer quantity (float)",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "90.5"},
			wantErrIs:   validator.ErrInvalidQuantityFormat,
			expectError: true,
		},
		{
			name:        "Non-integer quantity (string)",
			record:      []string{"evt-0001", "RELIANCE", "BUY", "abc"},
			wantErrIs:   validator.ErrInvalidQuantityFormat,
			expectError: true,
		},
		{
			name:        "Missing columns",
			record:      []string{"evt-0001", "RELIANCE"},
			wantErrIs:   validator.ErrInvalidColumnCount,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvent, err := validator.ValidateRecord(tt.record)
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
	validEvent := &models.OrderEvent{
		EventID:         "evt-100",
		Symbol:          "INFY",
		TransactionType: models.TxBuy,
		Quantity:        50,
	}
	if err := validator.ValidateEvent(validEvent); err != nil {
		t.Errorf("unexpected error for valid event: %v", err)
	}

	invalidEvent := &models.OrderEvent{
		EventID:         "evt-101",
		Symbol:          "INFY",
		TransactionType: "INVALID",
		Quantity:        50,
	}
	if err := validator.ValidateEvent(invalidEvent); err == nil {
		t.Errorf("expected error for invalid event transaction type, got nil")
	}
}
