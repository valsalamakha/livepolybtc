package execution

import (
	"math"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// SimulateMarketableBuy walks the ask side of book filling up to size at or
// below limitPrice, returning the filled quantity and the size-weighted average
// fill price. When the book is nil/empty it reports a zero fill so callers can
// decide how to treat missing liquidity.
func SimulateMarketableBuy(book *models.OrderBook, limitPrice, size float64) (filled, avgPrice float64) {
	if size <= 0 {
		return 0, 0
	}
	remaining := size
	var cost float64
	if book != nil {
		for _, lvl := range book.Asks {
			if remaining <= 0 || lvl.Price > limitPrice {
				break
			}
			take := math.Min(remaining, lvl.Size)
			cost += take * lvl.Price
			filled += take
			remaining -= take
		}
	}
	if filled <= 0 {
		return 0, 0
	}
	return filled, cost / filled
}
