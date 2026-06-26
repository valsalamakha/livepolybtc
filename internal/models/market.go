package models

import (
	"strings"
	"time"
)

// Token is a single tradable outcome (e.g. "Up" or "Down") within a market.
type Token struct {
	// TokenID is the ERC-1155 asset identifier used for trading and order
	// book subscriptions.
	TokenID string `json:"token_id"`
	// Outcome is the human-readable outcome label ("Up"/"Down", "Yes"/"No").
	Outcome string `json:"outcome"`
	// Price is the last known mid/market price for the token.
	Price float64 `json:"price"`
}

// Market describes a single Polymarket CLOB market.
type Market struct {
	// ConditionID is the on-chain condition identifier for the market.
	ConditionID string `json:"condition_id"`
	// QuestionID is the question identifier.
	QuestionID string `json:"question_id"`
	// Slug is the market URL slug.
	Slug string `json:"slug"`
	// Question is the human-readable question text.
	Question string `json:"question"`
	// Tokens are the tradable outcomes (binary markets have two).
	Tokens []Token `json:"tokens"`
	// Active reports whether the market is accepting orders.
	Active bool `json:"active"`
	// Closed reports whether the market has resolved/closed.
	Closed bool `json:"closed"`
	// AcceptingOrders reports whether the CLOB is currently accepting orders.
	AcceptingOrders bool `json:"accepting_orders"`
	// MinTickSize is the minimum price increment.
	MinTickSize float64 `json:"min_tick_size"`
	// MinOrderSize is the minimum order size in shares.
	MinOrderSize float64 `json:"min_order_size"`
	// StartTime is when the market opened / the observation period began.
	StartTime time.Time `json:"start_time"`
	// EndTime is when the market resolves.
	EndTime time.Time `json:"end_time"`
}

// TokenByOutcome returns the token whose outcome label matches (case
// insensitive), and whether it was found.
func (m *Market) TokenByOutcome(outcome string) (Token, bool) {
	for _, t := range m.Tokens {
		if strings.EqualFold(t.Outcome, outcome) {
			return t, true
		}
	}
	return Token{}, false
}

// TokenByID returns the token with the given id and whether it was found.
func (m *Market) TokenByID(id string) (Token, bool) {
	for _, t := range m.Tokens {
		if t.TokenID == id {
			return t, true
		}
	}
	return Token{}, false
}

// OppositeToken returns the other token in a binary market given one token id.
func (m *Market) OppositeToken(id string) (Token, bool) {
	if len(m.Tokens) != 2 {
		return Token{}, false
	}
	if m.Tokens[0].TokenID == id {
		return m.Tokens[1], true
	}
	if m.Tokens[1].TokenID == id {
		return m.Tokens[0], true
	}
	return Token{}, false
}

// TimeToResolution returns the duration until the market resolves relative to
// the provided time. It is negative once the market has resolved.
func (m *Market) TimeToResolution(now time.Time) time.Duration {
	return m.EndTime.Sub(now)
}

// TokenIDs returns the list of token ids for subscription purposes.
func (m *Market) TokenIDs() []string {
	ids := make([]string, 0, len(m.Tokens))
	for _, t := range m.Tokens {
		ids = append(ids, t.TokenID)
	}
	return ids
}
