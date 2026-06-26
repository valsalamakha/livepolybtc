// Package optimize performs grid-search optimization of strategy parameters by
// replaying a dataset through the backtester for every parameter combination
// and ranking the results by a configurable objective.
package optimize

import (
	"sort"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/backtest"
	"github.com/valsalamakha/livepolybtc/internal/config"
)

// Grid enumerates the candidate values for each sweepable parameter. Empty
// slices fall back to the base configuration's value for that parameter.
type Grid struct {
	MaximumBTCMove            []float64
	MinimumPriceSpike         []float64
	FinalObservationWindow    []time.Duration
	MinimumOrderBookImbalance []float64
	MinimumExpectedReturn     []float64
	MinimumLiquidity          []float64
	FixedSize                 []float64
	// LatencyAssumption shifts the marketable-limit buffer to model the cost
	// of acting later/with worse fills.
	MarketableLimitBuffer []float64
}

// Params is one concrete parameter combination.
type Params struct {
	MaximumBTCMove            float64       `json:"maximum_btc_move"`
	MinimumPriceSpike         float64       `json:"minimum_price_spike"`
	FinalObservationWindow    time.Duration `json:"final_observation_window"`
	MinimumOrderBookImbalance float64       `json:"minimum_order_book_imbalance"`
	MinimumExpectedReturn     float64       `json:"minimum_expected_return"`
	MinimumLiquidity          float64       `json:"minimum_liquidity"`
	FixedSize                 float64       `json:"fixed_size"`
	MarketableLimitBuffer     float64       `json:"marketable_limit_buffer"`
}

// Result couples a parameter set with its backtest metrics and objective score.
type Result struct {
	Params  Params           `json:"params"`
	Metrics backtest.Metrics `json:"metrics"`
	Score   float64          `json:"score"`
}

// Objective scores a metrics result; higher is better.
type Objective func(backtest.Metrics) float64

// NetPLObjective ranks by net profit.
func NetPLObjective(m backtest.Metrics) float64 { return m.NetPL }

// SharpeObjective ranks by per-trade Sharpe.
func SharpeObjective(m backtest.Metrics) float64 { return m.Sharpe }

// ExpectancyObjective ranks by per-trade expectancy.
func ExpectancyObjective(m backtest.Metrics) float64 { return m.Expectancy }

// Run executes the grid search and returns results sorted best-first. A result
// with zero trades is scored as -inf-equivalent so it sorts last.
func Run(base config.Config, ticks []backtest.Tick, grid Grid, obj Objective) []Result {
	if obj == nil {
		obj = NetPLObjective
	}
	g := grid.withDefaults(base)

	var results []Result
	for _, btcMove := range g.MaximumBTCMove {
		for _, spike := range g.MinimumPriceSpike {
			for _, window := range g.FinalObservationWindow {
				for _, imb := range g.MinimumOrderBookImbalance {
					for _, exp := range g.MinimumExpectedReturn {
						for _, liq := range g.MinimumLiquidity {
							for _, size := range g.FixedSize {
								for _, buf := range g.MarketableLimitBuffer {
									p := Params{
										MaximumBTCMove:            btcMove,
										MinimumPriceSpike:         spike,
										FinalObservationWindow:    window,
										MinimumOrderBookImbalance: imb,
										MinimumExpectedReturn:     exp,
										MinimumLiquidity:          liq,
										FixedSize:                 size,
										MarketableLimitBuffer:     buf,
									}
									cfg := applyParams(base, p)
									res := backtest.New(cfg).Run(ticks)
									score := obj(res.Metrics)
									if res.Metrics.Trades == 0 {
										score = -1e18
									}
									results = append(results, Result{Params: p, Metrics: res.Metrics, Score: score})
								}
							}
						}
					}
				}
			}
		}
	}

	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

func applyParams(base config.Config, p Params) config.Config {
	cfg := base // shallow copy is sufficient (value structs)
	cfg.Strategy.MaximumBTCMove = p.MaximumBTCMove
	cfg.Strategy.MinimumPriceSpike = p.MinimumPriceSpike
	cfg.Strategy.FinalObservationWindow = p.FinalObservationWindow
	cfg.Strategy.MinimumOrderBookImbalance = p.MinimumOrderBookImbalance
	cfg.Strategy.MinimumExpectedReturn = p.MinimumExpectedReturn
	cfg.Strategy.MinimumLiquidity = p.MinimumLiquidity
	cfg.Strategy.MaximumEntryPrice = 1.0 / p.MinimumExpectedReturn
	cfg.Sizing.FixedSize = p.FixedSize
	cfg.Execution.MarketableLimitBuffer = p.MarketableLimitBuffer
	return cfg
}

func (g Grid) withDefaults(base config.Config) Grid {
	out := g
	if len(out.MaximumBTCMove) == 0 {
		out.MaximumBTCMove = []float64{base.Strategy.MaximumBTCMove}
	}
	if len(out.MinimumPriceSpike) == 0 {
		out.MinimumPriceSpike = []float64{base.Strategy.MinimumPriceSpike}
	}
	if len(out.FinalObservationWindow) == 0 {
		out.FinalObservationWindow = []time.Duration{base.Strategy.FinalObservationWindow}
	}
	if len(out.MinimumOrderBookImbalance) == 0 {
		out.MinimumOrderBookImbalance = []float64{base.Strategy.MinimumOrderBookImbalance}
	}
	if len(out.MinimumExpectedReturn) == 0 {
		out.MinimumExpectedReturn = []float64{base.Strategy.MinimumExpectedReturn}
	}
	if len(out.MinimumLiquidity) == 0 {
		out.MinimumLiquidity = []float64{base.Strategy.MinimumLiquidity}
	}
	if len(out.FixedSize) == 0 {
		out.FixedSize = []float64{base.Sizing.FixedSize}
	}
	if len(out.MarketableLimitBuffer) == 0 {
		out.MarketableLimitBuffer = []float64{base.Execution.MarketableLimitBuffer}
	}
	return out
}
