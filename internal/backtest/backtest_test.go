package backtest

import (
	"math"
	"strings"
	"testing"

	"github.com/valsalamakha/livepolybtc/internal/config"
)

func TestGenerateSyntheticDeterministic(t *testing.T) {
	a := GenerateSynthetic(20, 99)
	b := GenerateSynthetic(20, 99)
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	if a[len(a)/2].BTCPrice != b[len(b)/2].BTCPrice {
		t.Error("synthetic data not deterministic for same seed")
	}
}

func TestBacktestProducesTrades(t *testing.T) {
	cfg := config.Default()
	cfg.ApplyDerived()
	ticks := GenerateSynthetic(100, 42)
	res := New(*cfg).Run(ticks)
	if res.Metrics.Trades == 0 {
		t.Fatal("expected the backtest to produce trades on the synthetic dataset")
	}
	if res.Metrics.Wins+res.Metrics.Losses != res.Metrics.Trades {
		t.Errorf("wins+losses (%d) != trades (%d)", res.Metrics.Wins+res.Metrics.Losses, res.Metrics.Trades)
	}
	if res.Metrics.WinRate < 0 || res.Metrics.WinRate > 1 {
		t.Errorf("win rate out of range: %v", res.Metrics.WinRate)
	}
}

func TestComputeMetricsBasic(t *testing.T) {
	trades := []TradeRecord{
		{EntryPrice: 0.05, Size: 100, Won: true, RealizedPL: 95, PayOff: 100}, // +95
		{EntryPrice: 0.05, Size: 100, Won: false, RealizedPL: -5, PayOff: 0},  // -5
		{EntryPrice: 0.05, Size: 100, Won: false, RealizedPL: -5, PayOff: 0},  // -5
	}
	m := ComputeMetrics(trades)
	if m.Trades != 3 || m.Wins != 1 || m.Losses != 2 {
		t.Errorf("counts wrong: %+v", m)
	}
	if math.Abs(m.NetPL-85) > 1e-9 {
		t.Errorf("net pl = %v, want 85", m.NetPL)
	}
	if math.Abs(m.WinRate-1.0/3.0) > 1e-9 {
		t.Errorf("win rate = %v, want 0.333", m.WinRate)
	}
	// gross profit 95, gross loss 10 => PF 9.5
	if math.Abs(m.ProfitFactor-9.5) > 1e-9 {
		t.Errorf("profit factor = %v, want 9.5", m.ProfitFactor)
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	m := ComputeMetrics(nil)
	if m.Trades != 0 || m.NetPL != 0 {
		t.Errorf("empty metrics wrong: %+v", m)
	}
}

func TestParseCSV(t *testing.T) {
	csv := strings.Join([]string{
		"timestamp,condition_id,end_time,up_token,down_token,up_bid,up_bid_size,up_ask,up_ask_size,down_bid,down_bid_size,down_ask,down_ask_size,btc_price",
		"2026-01-01T12:00:00Z,0xC,2026-01-01T12:05:00Z,UP,DN,0.84,5000,0.86,80,0.14,60,0.16,2000,60000",
	}, "\n")
	ticks, err := ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ticks) != 1 {
		t.Fatalf("got %d ticks, want 1", len(ticks))
	}
	tk := ticks[0]
	if tk.ConditionID != "0xC" || tk.UpToken != "UP" {
		t.Errorf("unexpected tick: %+v", tk)
	}
	if bb, ok := tk.UpBook.BestBid(); !ok || bb.Price != 0.84 {
		t.Errorf("up best bid wrong: %+v ok=%v", bb, ok)
	}
}
