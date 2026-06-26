package models

import "time"

// Trade is a single executed trade observed on a market.
type Trade struct {
	TokenID   string    `json:"token_id"`
	Price     float64   `json:"price"`
	Size      float64   `json:"size"`
	Side      Side      `json:"side"`
	Timestamp time.Time `json:"timestamp"`
}

// Notional returns the dollar value of the trade.
func (t Trade) Notional() float64 { return t.Price * t.Size }
