package models

import "time"

// Side represents the side of an order.
type Side string

const (
	// Buy indicates a buy (long) order.
	Buy Side = "BUY"
	// Sell indicates a sell (short) order.
	Sell Side = "SELL"
)

// OrderType represents the time-in-force / order semantics supported by the
// Polymarket CLOB.
type OrderType string

const (
	// GTC is a good-till-cancelled limit order.
	GTC OrderType = "GTC"
	// GTD is a good-till-date limit order.
	GTD OrderType = "GTD"
	// FOK is a fill-or-kill (marketable) order.
	FOK OrderType = "FOK"
	// FAK is a fill-and-kill (immediate-or-cancel) order.
	FAK OrderType = "FAK"
)

// OrderStatus describes the lifecycle state of an order.
type OrderStatus string

const (
	// StatusPending means the order has been created locally but not yet
	// acknowledged by the exchange.
	StatusPending OrderStatus = "PENDING"
	// StatusLive means the order is resting on the book.
	StatusLive OrderStatus = "LIVE"
	// StatusMatched means the order has been (partially or fully) matched.
	StatusMatched OrderStatus = "MATCHED"
	// StatusFilled means the order has been completely filled.
	StatusFilled OrderStatus = "FILLED"
	// StatusCancelled means the order was cancelled.
	StatusCancelled OrderStatus = "CANCELLED"
	// StatusRejected means the exchange rejected the order.
	StatusRejected OrderStatus = "REJECTED"
	// StatusFailed means submission failed (network/client error).
	StatusFailed OrderStatus = "FAILED"
)

// Order is a request to trade a single CLOB outcome token.
type Order struct {
	// ClientID is a locally generated identifier used to correlate the order
	// across the execution pipeline.
	ClientID string `json:"client_id"`
	// ExchangeID is the identifier assigned by the exchange once accepted.
	ExchangeID string `json:"exchange_id,omitempty"`
	// TokenID is the ERC-1155 outcome token (asset) identifier.
	TokenID string `json:"token_id"`
	// Side is BUY or SELL.
	Side Side `json:"side"`
	// Type is the order time-in-force.
	Type OrderType `json:"type"`
	// Price is the limit price in [0,1] (probability units / dollars).
	Price float64 `json:"price"`
	// Size is the number of outcome shares.
	Size float64 `json:"size"`
	// Status is the current lifecycle state.
	Status OrderStatus `json:"status"`
	// Expiration is the GTD expiry (unix seconds); zero when not applicable.
	Expiration int64 `json:"expiration,omitempty"`

	// CreatedAt is when the order was created locally.
	CreatedAt time.Time `json:"created_at"`
	// SubmittedAt is when the order was sent to the exchange.
	SubmittedAt time.Time `json:"submitted_at,omitempty"`
	// FilledSize is the cumulative filled quantity.
	FilledSize float64 `json:"filled_size"`
	// AvgFillPrice is the size-weighted average fill price.
	AvgFillPrice float64 `json:"avg_fill_price"`
}

// Notional returns the total dollar value of the order at its limit price.
func (o Order) Notional() float64 { return o.Price * o.Size }

// RemainingSize returns the unfilled portion of the order.
func (o Order) RemainingSize() float64 {
	r := o.Size - o.FilledSize
	if r < 0 {
		return 0
	}
	return r
}

// IsTerminal reports whether the order has reached a final state.
func (o Order) IsTerminal() bool {
	switch o.Status {
	case StatusFilled, StatusCancelled, StatusRejected, StatusFailed:
		return true
	default:
		return false
	}
}

// Fill is an execution against an order.
type Fill struct {
	OrderClientID string    `json:"order_client_id"`
	TradeID       string    `json:"trade_id"`
	TokenID       string    `json:"token_id"`
	Side          Side      `json:"side"`
	Price         float64   `json:"price"`
	Size          float64   `json:"size"`
	Timestamp     time.Time `json:"timestamp"`
}

// Notional returns the dollar value of the fill.
func (f Fill) Notional() float64 { return f.Price * f.Size }
