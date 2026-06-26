package execution

import (
	"context"
	"testing"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

func testExecutor() *Executor {
	cfg := config.Default().Execution
	log := logging.New(logging.LevelError, false)
	return NewExecutor(nil, NewUnsignedSigner("0xowner"), log, cfg, true)
}

func entryBook() *models.OrderBook {
	ob := &models.OrderBook{
		TokenID: "DOWN",
		Asks: []models.PriceLevel{
			{Price: 0.05, Size: 100},
			{Price: 0.06, Size: 500},
		},
	}
	ob.Normalize()
	return ob
}

func TestSimulatedFillFull(t *testing.T) {
	e := testExecutor()
	sig := &models.Signal{EntryTokenID: "DOWN", EntryOutcome: "Down", EntryPrice: 0.05, Side: models.Buy}
	res, err := e.Execute(context.Background(), sig, 200, entryBook())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.Simulated {
		t.Error("expected simulated result")
	}
	if res.FilledSize != 200 {
		t.Errorf("filled = %v, want 200", res.FilledSize)
	}
	// 100@0.05 + 100@0.06 = 11 / 200 = 0.055
	if res.AvgFillPrice < 0.0549 || res.AvgFillPrice > 0.0551 {
		t.Errorf("avg fill = %v, want ~0.055", res.AvgFillPrice)
	}
	if res.Order.Status != models.StatusFilled {
		t.Errorf("status = %v, want FILLED", res.Order.Status)
	}
}

func TestSimulatedPartialFill(t *testing.T) {
	e := testExecutor()
	// Marketable buffer (0.01) lifts limit to ~0.06, so only levels <= 0.06 fill.
	sig := &models.Signal{EntryTokenID: "DOWN", EntryPrice: 0.05, Side: models.Buy}
	res, err := e.Execute(context.Background(), sig, 1000, entryBook())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.FilledSize != 600 { // 100 + 500 available within limit
		t.Errorf("filled = %v, want 600 (partial)", res.FilledSize)
	}
	if res.Order.Status != models.StatusMatched {
		t.Errorf("status = %v, want MATCHED (partial)", res.Order.Status)
	}
}

func TestSimulatedNoBookFallsBackToSignalPrice(t *testing.T) {
	e := testExecutor()
	sig := &models.Signal{EntryTokenID: "DOWN", EntryPrice: 0.04, Side: models.Buy}
	res, err := e.Execute(context.Background(), sig, 100, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.AvgFillPrice != 0.04 || res.FilledSize != 100 {
		t.Errorf("fallback fill = %v @ %v, want 100 @ 0.04", res.FilledSize, res.AvgFillPrice)
	}
}

func TestMarketablePriceRounding(t *testing.T) {
	cfg := config.Default().Execution
	cfg.TickSize = 0.01
	cfg.MarketableLimitBuffer = 0.013
	e := NewExecutor(nil, NewUnsignedSigner(""), logging.New(logging.LevelError, false), cfg, true)
	// 0.05 + 0.013 = 0.063 -> rounds to 0.06
	if got := e.marketablePrice(0.05); got < 0.0599 || got > 0.0601 {
		t.Errorf("marketable price = %v, want ~0.06", got)
	}
	// Ceiling at 0.99
	if got := e.marketablePrice(0.995); got != 0.99 {
		t.Errorf("ceiling price = %v, want 0.99", got)
	}
}

func TestSimulatedAlwaysFlaggedSimulated(t *testing.T) {
	e := testExecutor()
	if !e.Simulated() {
		t.Error("unsigned + dry-run executor should be simulated")
	}
}

func TestSimulateMarketableBuyHelper(t *testing.T) {
	filled, avg := SimulateMarketableBuy(entryBook(), 0.06, 1000)
	if filled != 600 {
		t.Errorf("filled = %v, want 600", filled)
	}
	if avg < 0.0583 || avg > 0.0584 { // (100*.05 + 500*.06)/600 = 0.0583..
		t.Errorf("avg = %v, want ~0.0583", avg)
	}
}
