package strategy

import (
	"testing"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

func testCfg() config.StrategyConfig {
	c := config.Default()
	c.ApplyDerived()
	return c.Strategy
}

// book builds a single-level two-sided book around mid with given sizes.
func book(token string, ts time.Time, mid, bidSize, askSize float64) *models.OrderBook {
	ob := &models.OrderBook{
		TokenID:   token,
		Timestamp: ts,
		Bids:      []models.PriceLevel{{Price: mid - 0.01, Size: bidSize}},
		Asks:      []models.PriceLevel{{Price: mid + 0.01, Size: askSize}},
	}
	ob.Normalize()
	return ob
}

func mkMarket() *models.Market {
	return &models.Market{
		ConditionID: "0xCOND",
		EndTime:     time.Now(),
		Tokens: []models.Token{
			{TokenID: "UP", Outcome: "Up"},
			{TokenID: "DOWN", Outcome: "Down"},
		},
	}
}

func snapshot(m *models.Market, ts time.Time, ttr time.Duration, upMid, upBidSz, upAskSz, downMid, downBidSz, downAskSz, btcMove float64) models.Snapshot {
	return models.Snapshot{
		Market: m,
		Books: map[string]*models.OrderBook{
			"UP":   book("UP", ts, upMid, upBidSz, upAskSz),
			"DOWN": book("DOWN", ts, downMid, downBidSz, downAskSz),
		},
		BTC:              models.BTCStats{Price: 60000, Move5s: btcMove, Move20s: btcMove, Stale: false, Timestamp: ts},
		TimeToResolution: ttr,
		Timestamp:        ts,
	}
}

func TestEngineFiresOnOverreaction(t *testing.T) {
	eng := NewEngine(testCfg())
	m := mkMarket()
	now := time.Now()

	// Seed history: Up at 0.85 five seconds ago.
	pre := snapshot(m, now.Add(-5*time.Second), 15*time.Second, 0.85, 1000, 1000, 0.15, 1000, 1000, 3)
	eng.Observe(pre)

	// Now: Up spikes to 0.95 with strong buy imbalance; Down (entry) cheap and liquid.
	cur := snapshot(m, now, 10*time.Second, 0.95, 5000, 50, 0.05, 100, 2000, 3)
	sig, ok := eng.Evaluate(cur)
	if !ok {
		t.Fatal("expected a signal on overreaction")
	}
	if sig.EntryOutcome != "Down" {
		t.Errorf("entry outcome = %q, want Down (fade the spiked Up side)", sig.EntryOutcome)
	}
	if sig.EntryTokenID != "DOWN" {
		t.Errorf("entry token = %q, want DOWN", sig.EntryTokenID)
	}
	if sig.ExpectedReturn < testCfg().MinimumExpectedReturn {
		t.Errorf("expected return %.2f below minimum", sig.ExpectedReturn)
	}
	if sig.Strength <= 0 || sig.Strength > 1 {
		t.Errorf("strength %v out of (0,1]", sig.Strength)
	}
}

func TestEngineNoFireWhenBTCMoved(t *testing.T) {
	eng := NewEngine(testCfg())
	m := mkMarket()
	now := time.Now()
	eng.Observe(snapshot(m, now.Add(-5*time.Second), 15*time.Second, 0.85, 1000, 1000, 0.15, 1000, 1000, 3))

	// Big BTC move => the spike is justified, do not fade.
	cur := snapshot(m, now, 10*time.Second, 0.95, 5000, 50, 0.05, 100, 2000, 50)
	if _, ok := eng.Evaluate(cur); ok {
		t.Error("should not fire when BTC moved beyond threshold")
	}
}

func TestEngineNoFireOutsideWindow(t *testing.T) {
	eng := NewEngine(testCfg())
	m := mkMarket()
	now := time.Now()
	eng.Observe(snapshot(m, now.Add(-5*time.Second), 60*time.Second, 0.85, 1000, 1000, 0.15, 1000, 1000, 3))
	cur := snapshot(m, now, 45*time.Second, 0.95, 5000, 50, 0.05, 100, 2000, 3)
	if _, ok := eng.Evaluate(cur); ok {
		t.Error("should not fire outside the observation window")
	}
}

func TestEngineNoFireWithoutSpike(t *testing.T) {
	eng := NewEngine(testCfg())
	m := mkMarket()
	now := time.Now()
	// Up already high but flat (no spike).
	eng.Observe(snapshot(m, now.Add(-5*time.Second), 15*time.Second, 0.95, 5000, 50, 0.05, 100, 2000, 3))
	cur := snapshot(m, now, 10*time.Second, 0.95, 5000, 50, 0.05, 100, 2000, 3)
	if _, ok := eng.Evaluate(cur); ok {
		t.Error("should not fire without a price spike")
	}
}

func TestEngineNoFireWhenStale(t *testing.T) {
	eng := NewEngine(testCfg())
	m := mkMarket()
	now := time.Now()
	eng.Observe(snapshot(m, now.Add(-5*time.Second), 15*time.Second, 0.85, 1000, 1000, 0.15, 1000, 1000, 3))
	cur := snapshot(m, now, 10*time.Second, 0.95, 5000, 50, 0.05, 100, 2000, 3)
	cur.BTC.Stale = true
	if _, ok := eng.Evaluate(cur); ok {
		t.Error("should not fire when BTC feed is stale")
	}
}

func TestEngineNoFireLowLiquidity(t *testing.T) {
	eng := NewEngine(testCfg())
	m := mkMarket()
	now := time.Now()
	eng.Observe(snapshot(m, now.Add(-5*time.Second), 15*time.Second, 0.85, 1000, 1000, 0.15, 1000, 1000, 3))
	// Entry side has tiny ask depth (< minimum liquidity 50): 0.06*100 = 6.
	cur := snapshot(m, now, 10*time.Second, 0.95, 5000, 50, 0.05, 100, 100, 3)
	if _, ok := eng.Evaluate(cur); ok {
		t.Error("should not fire when entry liquidity is insufficient")
	}
}
