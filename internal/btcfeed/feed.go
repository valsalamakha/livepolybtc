// Package btcfeed monitors the live Bitcoin spot price from a configurable
// exchange and derives short-window statistics (velocity, 5s/20s volatility
// and move) used by the strategy to judge whether a market move is justified
// by the underlying.
package btcfeed

import (
	"context"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Feed polls a price Source and maintains rolling statistics. It is safe for
// concurrent use.
type Feed struct {
	src           Source
	pollInterval  time.Duration
	staleAfter    time.Duration
	historyWindow time.Duration

	mu      sync.RWMutex
	samples []models.BTCQuote
	last    time.Time

	// Updates receives a fresh BTCStats after every successful poll. It is
	// buffered and non-blocking; slow consumers drop intermediate values.
	Updates chan models.BTCStats

	clock func() time.Time
}

// New constructs a Feed from configuration. The http.Client may be nil.
func New(cfg config.BTCFeedConfig, hc *http.Client) (*Feed, error) {
	src, err := NewSource(cfg.Exchange, cfg.Symbol, hc)
	if err != nil {
		return nil, err
	}
	return NewWithSource(src, cfg), nil
}

// NewWithSource constructs a Feed around a caller-supplied Source. It is useful
// for testing and for plugging in alternative (e.g. websocket) sources.
func NewWithSource(src Source, cfg config.BTCFeedConfig) *Feed {
	pi := cfg.PollInterval
	if pi <= 0 {
		pi = 500 * time.Millisecond
	}
	return &Feed{
		src:           src,
		pollInterval:  pi,
		staleAfter:    cfg.StaleAfter,
		historyWindow: cfg.HistoryWindow,
		Updates:       make(chan models.BTCStats, 1),
		clock:         time.Now,
	}
}

// Exchange returns the underlying source name.
func (f *Feed) Exchange() string { return f.src.Name() }

// Run polls the source until the context is cancelled. Transient fetch errors
// are tolerated; the feed reports staleness via BTCStats.Stale.
func (f *Feed) Run(ctx context.Context) error {
	t := time.NewTicker(f.pollInterval)
	defer t.Stop()

	// Prime immediately so the first stats are available quickly.
	f.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			f.poll(ctx)
		}
	}
}

func (f *Feed) poll(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, f.pollInterval+2*time.Second)
	price, err := f.src.Fetch(cctx)
	cancel()
	if err != nil {
		return // tolerate; staleness will reflect the gap
	}
	now := f.clock()

	f.mu.Lock()
	f.samples = append(f.samples, models.BTCQuote{Exchange: f.src.Name(), Price: price, Timestamp: now})
	f.last = now
	cutoff := now.Add(-f.historyWindow)
	i := 0
	for i < len(f.samples) && f.samples[i].Timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		f.samples = f.samples[i:]
	}
	stats := f.computeLocked(now)
	f.mu.Unlock()

	select {
	case f.Updates <- stats:
	default:
		// Drop if the consumer is behind; Stats() always returns the latest.
		select {
		case <-f.Updates:
		default:
		}
		select {
		case f.Updates <- stats:
		default:
		}
	}
}

// Stats returns the current derived statistics.
func (f *Feed) Stats() models.BTCStats {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.computeLocked(f.clock())
}

// computeLocked computes statistics from the retained samples. Caller holds the
// lock (read or write).
func (f *Feed) computeLocked(now time.Time) models.BTCStats {
	n := len(f.samples)
	if n == 0 {
		return models.BTCStats{Timestamp: now, Stale: true}
	}
	latest := f.samples[n-1]
	stats := models.BTCStats{
		Price:     latest.Price,
		Timestamp: now,
		Stale:     now.Sub(f.last) > f.staleAfter,
	}

	stats.Move5s = f.moveOver(now, 5*time.Second)
	stats.Move20s = f.moveOver(now, 20*time.Second)
	stats.Volatility5s = f.volatilityOver(now, 5*time.Second)
	stats.Volatility20s = f.volatilityOver(now, 20*time.Second)
	stats.Velocity = f.velocity(now, time.Second)
	return stats
}

// priceAtOrBefore returns the most recent sampled price at or before target.
func (f *Feed) priceAtOrBefore(target time.Time) (float64, bool) {
	var price float64
	var found bool
	for _, s := range f.samples {
		if !s.Timestamp.After(target) {
			price = s.Price
			found = true
			continue
		}
		break
	}
	if !found && len(f.samples) > 0 {
		return f.samples[0].Price, true
	}
	return price, found
}

func (f *Feed) moveOver(now time.Time, d time.Duration) float64 {
	latest := f.samples[len(f.samples)-1].Price
	past, ok := f.priceAtOrBefore(now.Add(-d))
	if !ok {
		return 0
	}
	return math.Abs(latest - past)
}

func (f *Feed) velocity(now time.Time, d time.Duration) float64 {
	latest := f.samples[len(f.samples)-1].Price
	past, ok := f.priceAtOrBefore(now.Add(-d))
	if !ok || d <= 0 {
		return 0
	}
	return (latest - past) / d.Seconds()
}

func (f *Feed) volatilityOver(now time.Time, d time.Duration) float64 {
	cutoff := now.Add(-d)
	var sum, sumSq float64
	var count int
	for _, s := range f.samples {
		if s.Timestamp.Before(cutoff) {
			continue
		}
		sum += s.Price
		sumSq += s.Price * s.Price
		count++
	}
	if count < 2 {
		return 0
	}
	mean := sum / float64(count)
	variance := sumSq/float64(count) - mean*mean
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}
