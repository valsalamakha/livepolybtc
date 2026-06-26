package btcfeed

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
)

// stubSource returns a scripted sequence of prices.
type stubSource struct {
	mu     sync.Mutex
	prices []float64
	idx    int
}

func (s *stubSource) Name() string { return "stub" }

func (s *stubSource) Fetch(context.Context) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.prices[s.idx]
	if s.idx < len(s.prices)-1 {
		s.idx++
	}
	return p, nil
}

func TestFeedComputesMoveAndVelocity(t *testing.T) {
	src := &stubSource{prices: []float64{60000, 60010, 60020, 60030, 60040, 60050}}
	cfg := config.BTCFeedConfig{PollInterval: time.Second, StaleAfter: 3 * time.Second, HistoryWindow: 60 * time.Second}
	f := NewWithSource(src, cfg)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		f.clock = func() time.Time { return at }
		f.poll(context.Background())
	}

	stats := f.Stats()
	if math.Abs(stats.Price-60050) > 1e-9 {
		t.Errorf("price = %v, want 60050", stats.Price)
	}
	if math.Abs(stats.Move5s-50) > 1e-9 {
		t.Errorf("move5s = %v, want 50", stats.Move5s)
	}
	if math.Abs(stats.Velocity-10) > 1e-9 {
		t.Errorf("velocity = %v, want 10", stats.Velocity)
	}
	if stats.Stale {
		t.Error("feed should not be stale immediately after polling")
	}
	if stats.Volatility5s <= 0 {
		t.Error("expected positive 5s volatility for a moving price")
	}
}

func TestFeedStaleWhenNoData(t *testing.T) {
	src := &stubSource{prices: []float64{60000}}
	f := NewWithSource(src, config.BTCFeedConfig{PollInterval: time.Second, StaleAfter: time.Second, HistoryWindow: time.Minute})
	if !f.Stats().Stale {
		t.Error("expected stale stats before any poll")
	}
}

func TestSourceConstruction(t *testing.T) {
	for _, ex := range []string{"coinbase", "binance", "kraken"} {
		if _, err := NewSource(ex, "", nil); err != nil {
			t.Errorf("NewSource(%s): %v", ex, err)
		}
	}
	if _, err := NewSource("bogus", "", nil); err == nil {
		t.Error("expected error for unsupported exchange")
	}
}
