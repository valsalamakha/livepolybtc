package backtest

import (
	"math"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// btcTracker reproduces the live feed's statistics over replayed prices so the
// strategy sees consistent BTC move/volatility inputs during backtests.
type btcTracker struct {
	window     time.Duration
	staleAfter time.Duration
	samples    []models.BTCQuote
	last       time.Time
}

func newBTCTracker(window, staleAfter time.Duration) *btcTracker {
	if window <= 0 {
		window = 60 * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = 3 * time.Second
	}
	return &btcTracker{window: window, staleAfter: staleAfter}
}

func (b *btcTracker) add(t time.Time, price float64) {
	if price <= 0 {
		return
	}
	b.samples = append(b.samples, models.BTCQuote{Price: price, Timestamp: t})
	b.last = t
	cutoff := t.Add(-b.window)
	i := 0
	for i < len(b.samples) && b.samples[i].Timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		b.samples = b.samples[i:]
	}
}

func (b *btcTracker) stats(now time.Time) models.BTCStats {
	if len(b.samples) == 0 {
		return models.BTCStats{Timestamp: now, Stale: true}
	}
	latest := b.samples[len(b.samples)-1].Price
	return models.BTCStats{
		Price:         latest,
		Move5s:        b.move(now, 5*time.Second),
		Move20s:       b.move(now, 20*time.Second),
		Volatility5s:  b.vol(now, 5*time.Second),
		Volatility20s: b.vol(now, 20*time.Second),
		Velocity:      b.velocity(now, time.Second),
		Timestamp:     now,
		Stale:         now.Sub(b.last) > b.staleAfter,
	}
}

func (b *btcTracker) priceBefore(target time.Time) (float64, bool) {
	var price float64
	var found bool
	for _, s := range b.samples {
		if !s.Timestamp.After(target) {
			price = s.Price
			found = true
			continue
		}
		break
	}
	if !found && len(b.samples) > 0 {
		return b.samples[0].Price, true
	}
	return price, found
}

func (b *btcTracker) move(now time.Time, d time.Duration) float64 {
	latest := b.samples[len(b.samples)-1].Price
	past, ok := b.priceBefore(now.Add(-d))
	if !ok {
		return 0
	}
	return math.Abs(latest - past)
}

func (b *btcTracker) velocity(now time.Time, d time.Duration) float64 {
	latest := b.samples[len(b.samples)-1].Price
	past, ok := b.priceBefore(now.Add(-d))
	if !ok || d <= 0 {
		return 0
	}
	return (latest - past) / d.Seconds()
}

func (b *btcTracker) vol(now time.Time, d time.Duration) float64 {
	cutoff := now.Add(-d)
	var sum, sumSq float64
	var n int
	for _, s := range b.samples {
		if s.Timestamp.Before(cutoff) {
			continue
		}
		sum += s.Price
		sumSq += s.Price * s.Price
		n++
	}
	if n < 2 {
		return 0
	}
	mean := sum / float64(n)
	v := sumSq/float64(n) - mean*mean
	if v < 0 {
		v = 0
	}
	return math.Sqrt(v)
}
