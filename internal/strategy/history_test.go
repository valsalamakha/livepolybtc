package strategy

import (
	"testing"
	"time"
)

func TestPriceHistorySpike(t *testing.T) {
	h := newPriceHistory(30 * time.Second)
	base := time.Now()
	h.Add(base, 0.85)
	h.Add(base.Add(5*time.Second), 0.95)

	spike, ok := h.Spike(base.Add(5*time.Second), 5*time.Second)
	if !ok {
		t.Fatal("expected spike result")
	}
	if spike < 0.099 || spike > 0.101 {
		t.Errorf("spike = %v, want ~0.10", spike)
	}
}

func TestPriceHistoryEviction(t *testing.T) {
	h := newPriceHistory(10 * time.Second)
	base := time.Now()
	for i := 0; i < 20; i++ {
		h.Add(base.Add(time.Duration(i)*time.Second), float64(i))
	}
	// window is 10s, so older points evicted.
	if h.Len() > 12 {
		t.Errorf("history not evicted: len=%d", h.Len())
	}
	latest, ok := h.Latest()
	if !ok || latest != 19 {
		t.Errorf("latest = %v ok=%v, want 19", latest, ok)
	}
}

func TestPriceHistoryEmpty(t *testing.T) {
	h := newPriceHistory(time.Second)
	if _, ok := h.Latest(); ok {
		t.Error("expected no latest on empty history")
	}
	if _, ok := h.Spike(time.Now(), time.Second); ok {
		t.Error("expected no spike on empty history")
	}
}
