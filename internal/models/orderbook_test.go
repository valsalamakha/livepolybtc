package models

import (
	"math"
	"testing"
	"time"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func sampleBook() *OrderBook {
	ob := &OrderBook{
		TokenID:   "tok",
		Timestamp: time.Now(),
		Bids:      []PriceLevel{{0.40, 100}, {0.45, 200}}, // unsorted on purpose
		Asks:      []PriceLevel{{0.55, 150}, {0.50, 300}},
	}
	ob.Normalize()
	return ob
}

func TestNormalizeOrdersLevels(t *testing.T) {
	ob := sampleBook()
	bb, ok := ob.BestBid()
	if !ok || !approx(bb.Price, 0.45) {
		t.Fatalf("best bid = %+v ok=%v, want 0.45", bb, ok)
	}
	ba, ok := ob.BestAsk()
	if !ok || !approx(ba.Price, 0.50) {
		t.Fatalf("best ask = %+v ok=%v, want 0.50", ba, ok)
	}
}

func TestSpreadAndMid(t *testing.T) {
	ob := sampleBook()
	if got := ob.Spread(); !approx(got, 0.05) {
		t.Errorf("spread = %v, want 0.05", got)
	}
	if got := ob.MidPrice(); !approx(got, 0.475) {
		t.Errorf("mid = %v, want 0.475", got)
	}
}

func TestSpreadEmptySide(t *testing.T) {
	ob := &OrderBook{Bids: []PriceLevel{{0.4, 10}}}
	if got := ob.Spread(); !approx(got, 1.0) {
		t.Errorf("spread with empty ask = %v, want 1.0", got)
	}
}

func TestImbalance(t *testing.T) {
	ob := &OrderBook{
		Bids: []PriceLevel{{0.5, 1000}}, // bid notional 500
		Asks: []PriceLevel{{0.5, 100}},  // ask notional 50
	}
	ob.Normalize()
	// (500 - 50) / 550 = 0.8181...
	if got := ob.Imbalance(5); math.Abs(got-0.8181818) > 1e-4 {
		t.Errorf("imbalance = %v, want ~0.818", got)
	}
}

func TestDepthAndLiquidity(t *testing.T) {
	ob := sampleBook()
	// bid notional: 0.45*200 + 0.40*100 = 90 + 40 = 130
	if got := ob.BidDepth(5); !approx(got, 130) {
		t.Errorf("bid depth = %v, want 130", got)
	}
	// ask notional: 0.50*300 + 0.55*150 = 150 + 82.5 = 232.5
	if got := ob.AskDepth(5); !approx(got, 232.5) {
		t.Errorf("ask depth = %v, want 232.5", got)
	}
	if got := ob.Liquidity(5); !approx(got, 362.5) {
		t.Errorf("liquidity = %v, want 362.5", got)
	}
}

func TestDepthMaxLevels(t *testing.T) {
	ob := sampleBook()
	// only best bid: 0.45*200 = 90
	if got := ob.BidDepth(1); !approx(got, 90) {
		t.Errorf("bid depth(1) = %v, want 90", got)
	}
}

func TestClone(t *testing.T) {
	ob := sampleBook()
	cp := ob.Clone()
	cp.Bids[0].Size = 9999
	if ob.Bids[0].Size == 9999 {
		t.Error("clone shares backing array with original")
	}
}
