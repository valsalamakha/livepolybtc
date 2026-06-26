package models

import (
	"sort"
	"time"
)

// PriceLevel is a single level in an order book.
type PriceLevel struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

// OrderBook is a snapshot of resting liquidity for a single outcome token.
//
// Bids are sorted in descending price order (best bid first) and Asks are
// sorted in ascending price order (best ask first). Use Normalize after
// constructing a book from raw data to guarantee this invariant.
type OrderBook struct {
	TokenID   string       `json:"token_id"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp time.Time    `json:"timestamp"`
	Hash      string       `json:"hash,omitempty"`
}

// Normalize sorts the book so that the best bid and best ask are first.
func (ob *OrderBook) Normalize() {
	sort.SliceStable(ob.Bids, func(i, j int) bool { return ob.Bids[i].Price > ob.Bids[j].Price })
	sort.SliceStable(ob.Asks, func(i, j int) bool { return ob.Asks[i].Price < ob.Asks[j].Price })
}

// BestBid returns the highest bid level and whether one exists.
func (ob *OrderBook) BestBid() (PriceLevel, bool) {
	if len(ob.Bids) == 0 {
		return PriceLevel{}, false
	}
	return ob.Bids[0], true
}

// BestAsk returns the lowest ask level and whether one exists.
func (ob *OrderBook) BestAsk() (PriceLevel, bool) {
	if len(ob.Asks) == 0 {
		return PriceLevel{}, false
	}
	return ob.Asks[0], true
}

// Spread returns the absolute bid/ask spread. It returns 1.0 (the maximum
// possible spread in probability space) when one side of the book is empty.
func (ob *OrderBook) Spread() float64 {
	bid, okB := ob.BestBid()
	ask, okA := ob.BestAsk()
	if !okB || !okA {
		return 1.0
	}
	return ask.Price - bid.Price
}

// MidPrice returns the midpoint between best bid and best ask. It falls back to
// whichever side is populated, or 0 when the book is empty.
func (ob *OrderBook) MidPrice() float64 {
	bid, okB := ob.BestBid()
	ask, okA := ob.BestAsk()
	switch {
	case okB && okA:
		return (bid.Price + ask.Price) / 2
	case okB:
		return bid.Price
	case okA:
		return ask.Price
	default:
		return 0
	}
}

// ImpliedProbability returns the market-implied probability for this outcome,
// which for Polymarket binary markets is simply the mid price.
func (ob *OrderBook) ImpliedProbability() float64 { return ob.MidPrice() }

// BidDepth returns the cumulative notional resting on the bid side within
// maxLevels levels (use a non-positive value to include every level).
func (ob *OrderBook) BidDepth(maxLevels int) float64 { return depth(ob.Bids, maxLevels) }

// AskDepth returns the cumulative notional resting on the ask side within
// maxLevels levels.
func (ob *OrderBook) AskDepth(maxLevels int) float64 { return depth(ob.Asks, maxLevels) }

func depth(levels []PriceLevel, maxLevels int) float64 {
	var total float64
	for i, l := range levels {
		if maxLevels > 0 && i >= maxLevels {
			break
		}
		total += l.Price * l.Size
	}
	return total
}

// Imbalance returns the order-book imbalance in [-1,1] computed from the
// notional resting on each side within maxLevels. A value near +1 indicates
// overwhelming bid pressure, near -1 overwhelming ask pressure.
func (ob *OrderBook) Imbalance(maxLevels int) float64 {
	bid := ob.BidDepth(maxLevels)
	ask := ob.AskDepth(maxLevels)
	denom := bid + ask
	if denom == 0 {
		return 0
	}
	return (bid - ask) / denom
}

// Liquidity returns the total notional resting on both sides within maxLevels.
func (ob *OrderBook) Liquidity(maxLevels int) float64 {
	return ob.BidDepth(maxLevels) + ob.AskDepth(maxLevels)
}

// Clone returns a deep copy of the order book, safe to pass across goroutines.
func (ob *OrderBook) Clone() *OrderBook {
	if ob == nil {
		return nil
	}
	cp := &OrderBook{
		TokenID:   ob.TokenID,
		Timestamp: ob.Timestamp,
		Hash:      ob.Hash,
		Bids:      make([]PriceLevel, len(ob.Bids)),
		Asks:      make([]PriceLevel, len(ob.Asks)),
	}
	copy(cp.Bids, ob.Bids)
	copy(cp.Asks, ob.Asks)
	return cp
}
