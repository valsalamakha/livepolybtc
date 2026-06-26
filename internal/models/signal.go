package models

import "time"

// Snapshot bundles all the market data the strategy needs to evaluate a single
// decision point. It is produced by the market data layer and consumed by the
// strategy engine over a channel.
type Snapshot struct {
	Market           *Market               `json:"-"`
	Books            map[string]*OrderBook `json:"-"`
	LastTrades       map[string]Trade      `json:"-"`
	Volume           map[string]float64    `json:"-"`
	BTC              BTCStats              `json:"btc"`
	TimeToResolution time.Duration         `json:"time_to_resolution"`
	Timestamp        time.Time             `json:"timestamp"`
}

// Book returns the order book for a token id, or nil if absent.
func (s *Snapshot) Book(tokenID string) *OrderBook {
	if s.Books == nil {
		return nil
	}
	return s.Books[tokenID]
}

// Signal is a trade recommendation emitted by the strategy engine.
type Signal struct {
	// Timestamp is when the signal fired.
	Timestamp time.Time `json:"timestamp"`
	// Market is the market the signal applies to.
	ConditionID string `json:"condition_id"`
	// EntryTokenID is the token the bot should BUY (the cheap, faded side).
	EntryTokenID string `json:"entry_token_id"`
	// EntryOutcome is the human-readable outcome being entered.
	EntryOutcome string `json:"entry_outcome"`
	// OverreactedTokenID is the expensive token that overreacted.
	OverreactedTokenID string `json:"overreacted_token_id"`
	// EntryPrice is the price at which the bot intends to buy.
	EntryPrice float64 `json:"entry_price"`
	// Side is the side of the entry order (always Buy for this strategy).
	Side Side `json:"side"`
	// Strength is a normalized [0,1] confidence score.
	Strength float64 `json:"strength"`
	// ExpectedReturn is the payoff multiple (1/EntryPrice) if the entry wins.
	ExpectedReturn float64 `json:"expected_return"`
	// Reason is a human-readable explanation of why the signal fired.
	Reason string `json:"reason"`

	// Diagnostic fields captured at signal time for logging/backtesting.
	OverreactedPrice float64       `json:"overreacted_price"`
	PriceSpike       float64       `json:"price_spike"`
	BTCMove          float64       `json:"btc_move"`
	Imbalance        float64       `json:"imbalance"`
	Liquidity        float64       `json:"liquidity"`
	Spread           float64       `json:"spread"`
	TimeToResolution time.Duration `json:"time_to_resolution"`
}
