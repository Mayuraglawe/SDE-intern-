package validator

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"order-position-engine/internal/models"
)

var (
	ErrInvalidColumnCount   = errors.New("invalid CSV column count: expected 4 fields (event_id, symbol, transaction_type, quantity)")
	ErrBlankEventID         = errors.New("event_id cannot be blank")
	ErrBlankSymbol          = errors.New("symbol cannot be blank")
	ErrInvalidTxType        = errors.New("transaction_type must be exactly 'BUY' or 'SELL'")
	ErrInvalidQuantityFormat = errors.New("quantity must be a valid integer")
	ErrNonPositiveQuantity  = errors.New("quantity must be a strictly positive integer (> 0)")
)

// ValidateRecord validates a raw CSV row slice and returns a structured OrderEvent or error.
func ValidateRecord(record []string) (*models.OrderEvent, error) {
	if len(record) < 4 {
		return nil, ErrInvalidColumnCount
	}

	eventID := strings.TrimSpace(record[0])
	symbol := strings.TrimSpace(record[1])
	txTypeStr := strings.TrimSpace(record[2])
	qtyStr := strings.TrimSpace(record[3])

	if eventID == "" {
		return nil, ErrBlankEventID
	}
	if symbol == "" {
		return nil, ErrBlankSymbol
	}

	var txType models.TransactionType
	if txTypeStr == string(models.TxBuy) {
		txType = models.TxBuy
	} else if txTypeStr == string(models.TxSell) {
		txType = models.TxSell
	} else {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidTxType, txTypeStr)
	}

	qty, err := strconv.Atoi(qtyStr)
	if err != nil {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidQuantityFormat, qtyStr)
	}

	if qty <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrNonPositiveQuantity, qty)
	}

	return &models.OrderEvent{
		EventID:         eventID,
		Symbol:          symbol,
		TransactionType: txType,
		Quantity:        qty,
	}, nil
}

// ValidateEvent validates an unmarshaled OrderEvent struct.
func ValidateEvent(event *models.OrderEvent) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}

	if strings.TrimSpace(event.EventID) == "" {
		return ErrBlankEventID
	}
	if strings.TrimSpace(event.Symbol) == "" {
		return ErrBlankSymbol
	}
	if event.TransactionType != models.TxBuy && event.TransactionType != models.TxSell {
		return fmt.Errorf("%w: got %q", ErrInvalidTxType, event.TransactionType)
	}
	if event.Quantity <= 0 {
		return fmt.Errorf("%w: got %d", ErrNonPositiveQuantity, event.Quantity)
	}

	return nil
}
