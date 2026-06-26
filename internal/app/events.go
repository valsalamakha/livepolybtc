package app

import (
	"fmt"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/execution"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// recordSignal logs and records a fired signal.
func (b *Bot) recordSignal(sig *models.Signal, snap models.Snapshot) {
	bid, ask := bestBidAsk(snap.Book(sig.EntryTokenID))
	b.log.Debugf("SIGNAL strength=%.2f %s", sig.Strength, sig.Reason)
	b.rec.Record(logging.Event{
		Type:           logging.EventSignal,
		Market:         sig.ConditionID,
		BTCPrice:       snap.BTC.Price,
		BestBid:        bid,
		BestAsk:        ask,
		SignalStrength: sig.Strength,
		EntryReason:    sig.Reason,
		Side:           string(sig.Side),
		Price:          sig.EntryPrice,
		Message:        fmt.Sprintf("ttr=%s spike=%.3f imb=%.2f", sig.TimeToResolution.Round(time.Millisecond), sig.PriceSpike, sig.Imbalance),
	})
}

// recordFill logs and records an execution result.
func (b *Bot) recordFill(sig *models.Signal, res *execution.Result) {
	b.rec.Record(logging.Event{
		Type:        logging.EventFill,
		Market:      sig.ConditionID,
		EntryReason: sig.Reason,
		OrderID:     res.Order.ExchangeID,
		Side:        string(res.Order.Side),
		Price:       res.Order.Price,
		Size:        res.FilledSize,
		FillPrice:   res.AvgFillPrice,
		LatencyMS:   float64(res.Latency.Microseconds()) / 1000.0,
		Message:     fmt.Sprintf("slippage=%.4f simulated=%v status=%s", res.Slippage, res.Simulated, res.Order.Status),
	})
}

// recordOrderError records a failed execution attempt.
func (b *Bot) recordOrderError(sig *models.Signal, err error) {
	b.rec.Record(logging.Event{
		Type:        logging.EventReject,
		Market:      sig.ConditionID,
		EntryReason: sig.Reason,
		Side:        string(sig.Side),
		Price:       sig.EntryPrice,
		Message:     err.Error(),
	})
}

// recordSettle records a settled position.
func (b *Bot) recordSettle(pos *models.Position, realized float64) {
	b.rec.Record(logging.Event{
		Type:      logging.EventSettle,
		Market:    pos.ConditionID,
		OrderID:   pos.ID,
		Side:      "SETTLE",
		Price:     pos.EntryPrice,
		Size:      pos.Size,
		FillPrice: pos.ExitPrice,
		Profit:    realized,
		Message:   fmt.Sprintf("outcome=%s", pos.Outcome),
	})
}

func bestBidAsk(ob *models.OrderBook) (bid, ask float64) {
	if ob == nil {
		return 0, 0
	}
	if b, ok := ob.BestBid(); ok {
		bid = b.Price
	}
	if a, ok := ob.BestAsk(); ok {
		ask = a.Price
	}
	return bid, ask
}
