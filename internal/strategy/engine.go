// Package strategy implements the late-overreaction fade strategy.
//
// The hypothesis: in the final seconds before a Bitcoin 5-minute up/down
// market resolves, traders frequently pile aggressively into one side, pushing
// it to 90-99c. When that price spike is NOT justified by a corresponding move
// in the underlying BTC spot price, it is an overreaction. The bot fades it by
// buying the cheap opposite side, aiming to capture the reversal/settlement
// when the spike was unwarranted.
//
// Every gate is configurable so the parameters can be optimized later.
package strategy

import (
	"fmt"
	"math"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Engine evaluates market snapshots and emits trade signals. It is not safe
// for concurrent use; drive it from a single goroutine.
type Engine struct {
	cfg       config.StrategyConfig
	histories map[string]*priceHistory
}

// NewEngine constructs a strategy engine.
func NewEngine(cfg config.StrategyConfig) *Engine {
	return &Engine{
		cfg:       cfg,
		histories: make(map[string]*priceHistory),
	}
}

// Observe records the latest prices from a snapshot into the per-token
// histories. It must be called for every snapshot (including those outside the
// observation window) so the spike lookback has data.
func (e *Engine) Observe(snap models.Snapshot) {
	if snap.Market == nil {
		return
	}
	for _, tok := range snap.Market.Tokens {
		book := snap.Book(tok.TokenID)
		if book == nil {
			continue
		}
		mid := book.MidPrice()
		if mid <= 0 {
			continue
		}
		h, ok := e.histories[tok.TokenID]
		if !ok {
			// Retain a little more than the lookback to be safe.
			h = newPriceHistory(e.cfg.SpikeLookback + e.cfg.FinalObservationWindow)
			e.histories[tok.TokenID] = h
		}
		h.Add(snap.Timestamp, mid)
	}
}

// Evaluate observes the snapshot and returns a trade signal when all entry
// gates pass. The boolean reports whether a signal was produced.
func (e *Engine) Evaluate(snap models.Snapshot) (*models.Signal, bool) {
	e.Observe(snap)

	if snap.Market == nil || len(snap.Market.Tokens) != 2 {
		return nil, false
	}

	// Timing gate: only inside the final observation window, and not past the
	// entry deadline (too late to fill/settle).
	ttr := snap.TimeToResolution
	if ttr > e.cfg.FinalObservationWindow || ttr < e.cfg.EntryDeadline {
		return nil, false
	}

	// BTC feed must be fresh.
	if snap.BTC.Stale {
		return nil, false
	}

	btcMove := e.btcMoveForLookback(snap.BTC)

	// Examine each token as the potential "overreacted" (spiking) side.
	for _, tok := range snap.Market.Tokens {
		sig, ok := e.evaluateToken(snap, tok, btcMove)
		if ok {
			return sig, true
		}
	}
	return nil, false
}

func (e *Engine) evaluateToken(snap models.Snapshot, over models.Token, btcMove float64) (*models.Signal, bool) {
	overBook := snap.Book(over.TokenID)
	if overBook == nil {
		return nil, false
	}
	overPrice := overBook.MidPrice()

	// Gate 1: the overreacted side must be at high odds.
	if overPrice < e.cfg.MinimumOdds {
		return nil, false
	}

	// Gate 2: it must have spiked by at least MinimumPriceSpike over the
	// lookback window.
	hist := e.histories[over.TokenID]
	if hist == nil {
		return nil, false
	}
	spike, ok := hist.Spike(snap.Timestamp, e.cfg.SpikeLookback)
	if !ok || spike < e.cfg.MinimumPriceSpike {
		return nil, false
	}

	// Gate 3: BTC must have barely moved while odds exploded.
	if btcMove > e.cfg.MaximumBTCMove {
		return nil, false
	}

	// Gate 4: order-book imbalance on the overreacted side must confirm
	// aggressive one-sided buying pressure.
	imbalance := overBook.Imbalance(e.cfg.DepthLevels)
	if imbalance < e.cfg.MinimumOrderBookImbalance {
		return nil, false
	}

	// Identify the entry (faded) side.
	entryTok, ok := snap.Market.OppositeToken(over.TokenID)
	if !ok {
		return nil, false
	}
	entryBook := snap.Book(entryTok.TokenID)
	if entryBook == nil {
		return nil, false
	}

	// Entry price = best ask we must cross to buy the cheap side.
	ask, hasAsk := entryBook.BestAsk()
	if !hasAsk {
		return nil, false
	}
	entryPrice := ask.Price
	if entryPrice <= 0 {
		return nil, false
	}

	// Gate 5: spread on the entry side must be tight enough.
	if entryBook.Spread() > e.cfg.MaximumSpread {
		return nil, false
	}

	// Gate 6: enough liquidity to fill on the entry side.
	liquidity := entryBook.AskDepth(e.cfg.DepthLevels)
	if liquidity < e.cfg.MinimumLiquidity {
		return nil, false
	}

	// Gate 7: expected payoff multiple must clear the threshold.
	expectedReturn := 1.0 / entryPrice
	if expectedReturn < e.cfg.MinimumExpectedReturn || entryPrice > e.cfg.MaximumEntryPrice {
		return nil, false
	}

	sig := &models.Signal{
		Timestamp:          snap.Timestamp,
		ConditionID:        snap.Market.ConditionID,
		EntryTokenID:       entryTok.TokenID,
		EntryOutcome:       entryTok.Outcome,
		OverreactedTokenID: over.TokenID,
		EntryPrice:         entryPrice,
		Side:               models.Buy,
		ExpectedReturn:     expectedReturn,
		OverreactedPrice:   overPrice,
		PriceSpike:         spike,
		BTCMove:            btcMove,
		Imbalance:          imbalance,
		Liquidity:          liquidity,
		Spread:             entryBook.Spread(),
		TimeToResolution:   snap.TimeToResolution,
	}
	sig.Strength = e.strength(sig)
	sig.Reason = fmt.Sprintf(
		"%s spiked +%.0f¢ to %.0f¢ in %s while BTC moved $%.2f (<=$%.2f); fade by buying %s @ %.0f¢ (%.1fx payoff, imbalance %.2f)",
		over.Outcome, spike*100, overPrice*100, e.cfg.SpikeLookback, btcMove, e.cfg.MaximumBTCMove,
		entryTok.Outcome, entryPrice*100, expectedReturn, imbalance,
	)
	return sig, true
}

// btcMoveForLookback selects the BTC move statistic closest to the configured
// spike lookback window.
func (e *Engine) btcMoveForLookback(s models.BTCStats) float64 {
	if e.cfg.SpikeLookback <= 10*time.Second {
		return s.Move5s
	}
	return s.Move20s
}

// strength blends the qualifying factors into a normalized [0,1] confidence.
func (e *Engine) strength(s *models.Signal) float64 {
	// Spike component: how far past the minimum spike, capped.
	spikeComp := clamp01((s.PriceSpike - e.cfg.MinimumPriceSpike) / 0.15)
	// Imbalance component: how far past the minimum, scaled to remaining room.
	room := 1.0 - e.cfg.MinimumOrderBookImbalance
	imbComp := 0.0
	if room > 0 {
		imbComp = clamp01((s.Imbalance - e.cfg.MinimumOrderBookImbalance) / room)
	}
	// BTC quietness: the less BTC moved relative to the max, the stronger.
	btcComp := clamp01(1.0 - s.BTCMove/math.Max(e.cfg.MaximumBTCMove, 1e-9))
	// Payoff component: how far past the minimum expected return, capped.
	payoffComp := clamp01((s.ExpectedReturn - e.cfg.MinimumExpectedReturn) / 10.0)

	return 0.30*spikeComp + 0.30*imbComp + 0.25*btcComp + 0.15*payoffComp
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
