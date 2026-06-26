// Package execution implements the order execution engine: order construction,
// marketable-limit pricing, slippage estimation, submission with retries and
// timeout handling, partial-fill tracking, and a fully simulated execution
// path for dry-run/paper trading and backtesting.
package execution

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Result summarizes the outcome of an execution attempt.
type Result struct {
	Order        models.Order
	Fills        []models.Fill
	FilledSize   float64
	AvgFillPrice float64
	Slippage     float64
	Latency      time.Duration
	Simulated    bool
}

// Notional returns the dollar cost of the fills.
func (r Result) Notional() float64 { return r.AvgFillPrice * r.FilledSize }

// Executor places orders against the CLOB or simulates them when live signing
// is unavailable / dry-run is enabled.
type Executor struct {
	client *api.Client
	signer OrderSigner
	log    *logging.Logger
	cfg    config.ExecutionConfig
	dryRun bool
	clock  func() time.Time
}

// NewExecutor constructs an Executor. When dryRun is true, or the signer cannot
// sign, orders are simulated against the provided book rather than submitted.
func NewExecutor(client *api.Client, signer OrderSigner, log *logging.Logger, cfg config.ExecutionConfig, dryRun bool) *Executor {
	return &Executor{
		client: client,
		signer: signer,
		log:    log,
		cfg:    cfg,
		dryRun: dryRun,
		clock:  time.Now,
	}
}

// Simulated reports whether the executor will simulate rather than submit.
func (e *Executor) Simulated() bool {
	return e.dryRun || e.signer == nil || !e.signer.Available()
}

// Execute attempts to buy sizeShares of the signal's entry token using a
// marketable limit order. entryBook is the current book for the entry token
// and is used for marketable pricing, slippage estimation, and simulation.
func (e *Executor) Execute(ctx context.Context, sig *models.Signal, sizeShares float64, entryBook *models.OrderBook) (*Result, error) {
	if sizeShares <= 0 {
		return nil, fmt.Errorf("non-positive size")
	}
	limitPrice := e.marketablePrice(sig.EntryPrice)

	order := models.Order{
		ClientID:  newClientID(),
		TokenID:   sig.EntryTokenID,
		Side:      models.Buy,
		Type:      models.OrderType(e.cfg.OrderType),
		Price:     limitPrice,
		Size:      sizeShares,
		Status:    models.StatusPending,
		CreatedAt: e.clock(),
	}

	if e.Simulated() {
		return e.simulate(order, sig, entryBook), nil
	}
	return e.submitLive(ctx, order, sig)
}

// marketablePrice rounds the crossing price (best signal price + buffer) to the
// configured tick size, capped at the 0.99 ceiling.
func (e *Executor) marketablePrice(base float64) float64 {
	p := base + e.cfg.MarketableLimitBuffer
	p = roundToTick(p, e.cfg.TickSize)
	if p > 0.99 {
		p = 0.99
	}
	if p < e.cfg.TickSize {
		p = e.cfg.TickSize
	}
	return p
}

// simulate fills the order against the entry book's asks, modelling partial
// fills and slippage. It never sends anything to the exchange.
func (e *Executor) simulate(order models.Order, sig *models.Signal, book *models.OrderBook) *Result {
	start := e.clock()
	res := &Result{Order: order, Simulated: true}

	filled, avg := SimulateMarketableBuy(book, order.Price, order.Size)
	if filled <= 0 {
		// No book / no liquidity: assume we get the signalled price (paper).
		filled = order.Size
		avg = sig.EntryPrice
	}

	res.FilledSize = filled
	res.AvgFillPrice = avg
	res.Slippage = avg - sig.EntryPrice
	res.Latency = e.clock().Sub(start)

	res.Order.FilledSize = filled
	res.Order.AvgFillPrice = avg
	res.Order.SubmittedAt = start
	if filled >= order.Size-1e-9 {
		res.Order.Status = models.StatusFilled
	} else {
		res.Order.Status = models.StatusMatched
	}
	res.Fills = []models.Fill{{
		OrderClientID: order.ClientID,
		TokenID:       order.TokenID,
		Side:          models.Buy,
		Price:         avg,
		Size:          filled,
		Timestamp:     e.clock(),
	}}
	e.log.Debugf("simulated fill: %.4f @ %.4f (slippage %.4f)", filled, avg, res.Slippage)
	return res
}

// submitLive builds, signs and submits a real order with retry and timeout
// handling, then cancels any unfilled remainder.
func (e *Executor) submitLive(ctx context.Context, order models.Order, sig *models.Signal) (*Result, error) {
	start := e.clock()
	req, err := e.signer.SignOrder(order)
	if err != nil {
		return nil, fmt.Errorf("sign order: %w", err)
	}
	req.OrderType = e.cfg.OrderType
	if req.Owner == "" {
		req.Owner = e.signer.Owner()
	}

	var resp *api.OrderResponse
	backoff := e.cfg.RetryBackoff
	for attempt := 0; attempt <= e.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}
		cctx, cancel := context.WithTimeout(ctx, e.cfg.OrderTimeout)
		resp, err = e.client.PlaceOrder(cctx, req)
		cancel()
		if err == nil {
			break
		}
		e.log.Warnf("place order attempt %d failed: %v", attempt+1, err)
	}
	if err != nil {
		order.Status = models.StatusFailed
		return &Result{Order: order, Latency: e.clock().Sub(start)}, fmt.Errorf("submit order: %w", err)
	}

	order.ExchangeID = resp.OrderID
	filled := api2float(resp.MakingAmount, resp.TakingAmount)
	res := &Result{
		Order:      order,
		FilledSize: filled,
		Latency:    e.clock().Sub(start),
	}
	if filled > 0 {
		res.AvgFillPrice = sig.EntryPrice // best-effort; refined by user feed
		res.Slippage = res.AvgFillPrice - sig.EntryPrice
		order.FilledSize = filled
		order.AvgFillPrice = res.AvgFillPrice
	}
	switch resp.Status {
	case "matched", "MATCHED", "FILLED", "filled":
		order.Status = models.StatusFilled
	case "live", "LIVE":
		order.Status = models.StatusLive
	default:
		order.Status = models.StatusMatched
	}
	res.Order = order
	return res, nil
}

// Cancel cancels a resting order by exchange id.
func (e *Executor) Cancel(ctx context.Context, exchangeID string) error {
	if e.Simulated() || exchangeID == "" {
		return nil
	}
	return e.client.CancelOrder(ctx, exchangeID)
}

func roundToTick(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}
	return math.Round(price/tick) * tick
}

func newClientID() string { return newID() }

// api2float derives a filled-share quantity from the response amounts when
// present; returns 0 when unknown.
func api2float(making, taking string) float64 {
	// For a BUY, takingAmount is the shares received (6dp). Fall back to 0.
	if taking == "" {
		return 0
	}
	var v float64
	_, err := fmt.Sscanf(taking, "%f", &v)
	if err != nil {
		return 0
	}
	return v / 1e6
}
