package strategy

import (
	"time"
)

// pricePoint is a single (time, price) observation.
type pricePoint struct {
	t     time.Time
	price float64
}

// priceHistory is a bounded, time-windowed series of price observations for a
// single token. It is used to detect abnormal short-window price spikes.
type priceHistory struct {
	points []pricePoint
	window time.Duration
}

func newPriceHistory(window time.Duration) *priceHistory {
	return &priceHistory{window: window}
}

// Add records an observation and evicts points older than the retention
// window.
func (h *priceHistory) Add(t time.Time, price float64) {
	h.points = append(h.points, pricePoint{t: t, price: price})
	cutoff := t.Add(-h.window)
	// Evict from the front.
	i := 0
	for i < len(h.points) && h.points[i].t.Before(cutoff) {
		i++
	}
	if i > 0 {
		h.points = h.points[i:]
	}
}

// PriceAt returns the most recent price at or before the target time, and
// whether one exists.
func (h *priceHistory) PriceAt(target time.Time) (float64, bool) {
	var found bool
	var price float64
	for _, p := range h.points {
		if !p.t.After(target) {
			price = p.price
			found = true
			continue
		}
		break
	}
	if !found && len(h.points) > 0 {
		// No point old enough; use the oldest available.
		return h.points[0].price, true
	}
	return price, found
}

// Latest returns the most recent price and whether one exists.
func (h *priceHistory) Latest() (float64, bool) {
	if len(h.points) == 0 {
		return 0, false
	}
	return h.points[len(h.points)-1].price, true
}

// Spike returns the change in price over the lookback window ending at now
// (latest - price[now-lookback]). A positive value indicates the price rose.
func (h *priceHistory) Spike(now time.Time, lookback time.Duration) (float64, bool) {
	latest, ok := h.Latest()
	if !ok {
		return 0, false
	}
	past, ok := h.PriceAt(now.Add(-lookback))
	if !ok {
		return 0, false
	}
	return latest - past, true
}

// Len returns the number of retained points.
func (h *priceHistory) Len() int { return len(h.points) }
