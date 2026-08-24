package position_test

import (
	"fmt"
	"sync"
	"testing"

	"order-position-engine/internal/position"
	"order-position-engine/internal/shared"
)

func TestPositionManager_SingleSymbol(t *testing.T) {
	mgr := position.NewManager()

	// 1. BUY 90 RELIANCE
	res, err := mgr.ProcessEvent(shared.OrderEvent{
		EventID:         "evt-0001",
		Symbol:          "RELIANCE",
		TransactionType: shared.TxBuy,
		Quantity:        90,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != shared.StatusAccepted || res.NetPosition != 90 {
		t.Errorf("got status=%s net=%d, want ACCEPTED net=90", res.Status, res.NetPosition)
	}

	// 2. SELL 30 RELIANCE -> Net: 60
	res2, err := mgr.ProcessEvent(shared.OrderEvent{
		EventID:         "evt-0002",
		Symbol:          "RELIANCE",
		TransactionType: shared.TxSell,
		Quantity:        30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.NetPosition != 60 {
		t.Errorf("got net=%d, want 60", res2.NetPosition)
	}

	// 3. SELL 60 RELIANCE -> Net: 0 (must stay in GET /position)
	res3, err := mgr.ProcessEvent(shared.OrderEvent{
		EventID:         "evt-0003",
		Symbol:          "RELIANCE",
		TransactionType: shared.TxSell,
		Quantity:        60,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res3.NetPosition != 0 {
		t.Errorf("got net=%d, want 0", res3.NetPosition)
	}

	positions := mgr.GetPositions()
	val, ok := positions["RELIANCE"]
	if !ok {
		t.Fatalf("expected symbol RELIANCE in positions map even with net position 0")
	}
	if val != 0 {
		t.Errorf("got position %d, want 0", val)
	}
}

func TestPositionManager_NegativePosition(t *testing.T) {
	mgr := position.NewManager()

	// SELL 75 TCS on empty position -> Net: -75
	res, err := mgr.ProcessEvent(shared.OrderEvent{
		EventID:         "evt-0002",
		Symbol:          "TCS",
		TransactionType: shared.TxSell,
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NetPosition != -75 {
		t.Errorf("got net=%d, want -75", res.NetPosition)
	}

	positions := mgr.GetPositions()
	if positions["TCS"] != -75 {
		t.Errorf("got TCS position %d, want -75", positions["TCS"])
	}
}

func TestPositionManager_Idempotency(t *testing.T) {
	mgr := position.NewManager()

	// First event: BUY 100 INFY
	res1, err := mgr.ProcessEvent(shared.OrderEvent{
		EventID:         "evt-dup-001",
		Symbol:          "INFY",
		TransactionType: shared.TxBuy,
		Quantity:        100,
	})
	if err != nil || res1.Status != shared.StatusAccepted {
		t.Fatalf("first event failed: %v", err)
	}

	// Duplicate event: Same event_id, different quantity (200) -> First wins!
	res2, err := mgr.ProcessEvent(shared.OrderEvent{
		EventID:         "evt-dup-001",
		Symbol:          "INFY",
		TransactionType: shared.TxBuy,
		Quantity:        200,
	})
	if err != nil {
		t.Fatalf("duplicate event returned unexpected error: %v", err)
	}
	if res2.Status != shared.StatusDuplicate {
		t.Errorf("got status=%s, want DUPLICATE", res2.Status)
	}

	positions := mgr.GetPositions()
	if positions["INFY"] != 100 {
		t.Errorf("got INFY position %d, want 100 (duplicate should be ignored)", positions["INFY"])
	}
}

func TestPositionManager_ConcurrentAccess(t *testing.T) {
	mgr := position.NewManager()
	var wg sync.WaitGroup

	numGoroutines := 50
	eventsPerGoroutine := 20

	// Launch concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gId int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				eventID := fmt.Sprintf("evt-%d-%d", gId, j)
				symbol := fmt.Sprintf("SYM-%d", j%5)
				mgr.ProcessEvent(shared.OrderEvent{
					EventID:         eventID,
					Symbol:          symbol,
					TransactionType: shared.TxBuy,
					Quantity:        10,
				})
			}
		}(i)
	}

	// Launch concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = mgr.GetPositions()
			}
		}()
	}

	wg.Wait()

	positions := mgr.GetPositions()
	totalSymbols, uniqueEvents := mgr.Stats()

	if totalSymbols != 5 {
		t.Errorf("got %d total symbols, want 5", totalSymbols)
	}
	expectedUnique := numGoroutines * eventsPerGoroutine
	if uniqueEvents != expectedUnique {
		t.Errorf("got %d unique events, want %d", uniqueEvents, expectedUnique)
	}
	_ = positions
}
