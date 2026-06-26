package models

import "time"

// BTCQuote is a single observation of the BTC spot price from an exchange feed.
type BTCQuote struct {
	Exchange  string    `json:"exchange"`
	Price     float64   `json:"price"`
	Timestamp time.Time `json:"timestamp"`
}

// BTCStats are derived statistics about recent BTC price action used by the
// strategy to decide whether a market move is justified by the underlying.
type BTCStats struct {
	// Price is the most recent spot price.
	Price float64 `json:"price"`
	// Velocity is the price change per second over the short lookback window
	// (dollars/second).
	Velocity float64 `json:"velocity"`
	// Volatility5s is the standard deviation of prices over the last 5 seconds.
	Volatility5s float64 `json:"volatility_5s"`
	// Volatility20s is the standard deviation of prices over the last 20 seconds.
	Volatility20s float64 `json:"volatility_20s"`
	// Move5s is the absolute price change over the last 5 seconds.
	Move5s float64 `json:"move_5s"`
	// Move20s is the absolute price change over the last 20 seconds.
	Move20s float64 `json:"move_20s"`
	// Timestamp is when the stats were computed.
	Timestamp time.Time `json:"timestamp"`
	// Stale reports whether the feed has not updated recently enough to trust.
	Stale bool `json:"stale"`
}
