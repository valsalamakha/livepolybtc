package backtest

import (
	"math"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/execution"
	"github.com/valsalamakha/livepolybtc/internal/strategy"
)

// TradeRecord is a single completed backtest trade.
type TradeRecord struct {
	ConditionID string
	Outcome     string
	EntryTime   time.Time
	EntryPrice  float64
	Size        float64
	Won         bool
	PayOff      float64
	RealizedPL  float64
	Strength    float64
	Reason      string
}

// Cost returns the premium paid.
func (t TradeRecord) Cost() float64 { return t.EntryPrice * t.Size }

// ReturnPct returns the trade return as a fraction of premium.
func (t TradeRecord) ReturnPct() float64 {
	if c := t.Cost(); c > 0 {
		return t.RealizedPL / c
	}
	return 0
}

// Result is the outcome of a backtest run.
type Result struct {
	Trades  []TradeRecord
	Metrics Metrics
}

// Backtester replays ticks through the strategy and a simulated executor.
type Backtester struct {
	cfg config.Config
}

// New constructs a Backtester from a full configuration (strategy + sizing).
func New(cfg config.Config) *Backtester { return &Backtester{cfg: cfg} }

// Run replays the ticks and returns trades and computed metrics.
func (b *Backtester) Run(ticks []Tick) Result {
	if len(ticks) == 0 {
		return Result{Metrics: ComputeMetrics(nil)}
	}

	engine := strategy.NewEngine(b.cfg.Strategy)
	sizer := newSizer(b.cfg.Sizing)
	btc := newBTCTracker(b.cfg.BTCFeed.HistoryWindow, b.cfg.BTCFeed.StaleAfter)
	winners := computeWinners(ticks)

	var trades []TradeRecord
	open := map[string]*openTrade{} // conditionID -> open trade
	var lastCondition string

	settle := func(condition string) {
		ot := open[condition]
		if ot == nil {
			return
		}
		won := winners[condition] == ot.tokenID
		payoff := 0.0
		if won {
			payoff = ot.size
		}
		trades = append(trades, TradeRecord{
			ConditionID: condition,
			Outcome:     ot.outcome,
			EntryTime:   ot.entryTime,
			EntryPrice:  ot.entryPrice,
			Size:        ot.size,
			Won:         won,
			PayOff:      payoff,
			RealizedPL:  payoff - ot.entryPrice*ot.size,
			Strength:    ot.strength,
			Reason:      ot.reason,
		})
		delete(open, condition)
	}

	bankroll := b.cfg.Sizing.Bankroll
	for _, tk := range ticks {
		// Settle the previous market when the condition rolls over.
		if lastCondition != "" && tk.ConditionID != lastCondition {
			settle(lastCondition)
			engine = strategy.NewEngine(b.cfg.Strategy) // reset per-market state
		}
		lastCondition = tk.ConditionID

		btc.add(tk.Time, tk.BTCPrice)
		snap := tk.Snapshot(btc.stats(tk.Time))

		sig, ok := engine.Evaluate(snap)
		if !ok {
			continue
		}
		if _, exists := open[tk.ConditionID]; exists {
			continue // one position per market
		}

		notional := sizer.notional(bankroll, sig.EntryPrice)
		if notional <= 0 {
			continue
		}
		size := notional / sig.EntryPrice
		entryBook := snap.Book(sig.EntryTokenID)
		filled, avg := execution.SimulateMarketableBuy(entryBook, sig.EntryPrice+b.cfg.Execution.MarketableLimitBuffer, size)
		if filled <= 0 {
			filled, avg = size, sig.EntryPrice
		}
		open[tk.ConditionID] = &openTrade{
			tokenID:    sig.EntryTokenID,
			outcome:    sig.EntryOutcome,
			entryTime:  tk.Time,
			entryPrice: avg,
			size:       filled,
			strength:   sig.Strength,
			reason:     sig.Reason,
		}
	}
	// Settle any still-open market at end of data.
	if lastCondition != "" {
		settle(lastCondition)
	}

	return Result{Trades: trades, Metrics: ComputeMetrics(trades)}
}

type openTrade struct {
	tokenID    string
	outcome    string
	entryTime  time.Time
	entryPrice float64
	size       float64
	strength   float64
	reason     string
}

// computeWinners determines the winning token id per condition as the token
// whose mid is >= 0.5 at the final tick of that condition.
func computeWinners(ticks []Tick) map[string]string {
	last := map[string]Tick{}
	for _, tk := range ticks {
		last[tk.ConditionID] = tk
	}
	winners := make(map[string]string, len(last))
	for cond, tk := range last {
		upMid, downMid := 0.0, 0.0
		if tk.UpBook != nil {
			upMid = tk.UpBook.MidPrice()
		}
		if tk.DownBook != nil {
			downMid = tk.DownBook.MidPrice()
		}
		if upMid >= downMid {
			winners[cond] = tk.UpToken
		} else {
			winners[cond] = tk.DownToken
		}
	}
	return winners
}

// --- lightweight sizer (mirrors risk.Sizer without the locking) ---

type sizer struct{ cfg config.SizingConfig }

func newSizer(cfg config.SizingConfig) *sizer { return &sizer{cfg: cfg} }

func (s *sizer) notional(bankroll, entryPrice float64) float64 {
	var raw float64
	switch s.cfg.Mode {
	case config.SizePercent:
		raw = bankroll * s.cfg.PercentOfBankroll
	case config.SizeKelly:
		p := s.cfg.WinProbability
		if p > 0 && p < 1 && entryPrice > 0 && entryPrice < 1 {
			b := (1 - entryPrice) / entryPrice
			f := (p*b - (1 - p)) / b
			raw = bankroll * math.Max(0, math.Min(1, f*s.cfg.KellyFraction))
		}
	default:
		raw = s.cfg.FixedSize
	}
	if raw > s.cfg.MaxTradeSize {
		raw = s.cfg.MaxTradeSize
	}
	if raw < s.cfg.MinTradeSize {
		return 0
	}
	return raw
}
