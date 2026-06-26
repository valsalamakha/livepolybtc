package models

import "time"

// Position tracks an open or closed exposure resulting from a filled entry.
type Position struct {
	ID          string    `json:"id"`
	ConditionID string    `json:"condition_id"`
	TokenID     string    `json:"token_id"`
	Outcome     string    `json:"outcome"`
	Size        float64   `json:"size"`
	EntryPrice  float64   `json:"entry_price"`
	EntryTime   time.Time `json:"entry_time"`

	// Closed/settled fields.
	Closed     bool      `json:"closed"`
	ExitPrice  float64   `json:"exit_price"`
	ExitTime   time.Time `json:"exit_time"`
	RealizedPL float64   `json:"realized_pl"`
}

// Cost returns the premium paid to open the position.
func (p Position) Cost() float64 { return p.EntryPrice * p.Size }

// MaxPayoff returns the payoff if the position resolves in the money (each
// share pays out $1 on Polymarket binary markets).
func (p Position) MaxPayoff() float64 { return p.Size }

// UnrealizedPL returns the marked-to-market profit at the given price.
func (p Position) UnrealizedPL(mark float64) float64 {
	return (mark - p.EntryPrice) * p.Size
}
