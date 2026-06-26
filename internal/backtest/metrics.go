package backtest

import (
	"fmt"
	"math"
)

// Metrics summarizes backtest performance.
type Metrics struct {
	Trades         int     `json:"trades"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
	WinRate        float64 `json:"win_rate"`
	GrossProfit    float64 `json:"gross_profit"`
	GrossLoss      float64 `json:"gross_loss"`
	NetPL          float64 `json:"net_pl"`
	ProfitFactor   float64 `json:"profit_factor"`
	AvgReturn      float64 `json:"avg_return"`   // mean per-trade return on premium
	Expectancy     float64 `json:"expectancy"`   // mean per-trade PnL ($)
	Sharpe         float64 `json:"sharpe"`       // per-trade Sharpe of returns
	MaxDrawdown    float64 `json:"max_drawdown"` // peak-to-trough of equity ($)
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
}

// ComputeMetrics derives performance statistics from completed trades.
func ComputeMetrics(trades []TradeRecord) Metrics {
	m := Metrics{Trades: len(trades)}
	if len(trades) == 0 {
		return m
	}

	returns := make([]float64, 0, len(trades))
	var sumPL, sumRet float64
	for _, t := range trades {
		if t.Won {
			m.Wins++
			m.GrossProfit += t.RealizedPL
		} else {
			m.Losses++
			m.GrossLoss += -t.RealizedPL
		}
		sumPL += t.RealizedPL
		r := t.ReturnPct()
		sumRet += r
		returns = append(returns, r)
	}

	n := float64(len(trades))
	m.NetPL = sumPL
	m.WinRate = float64(m.Wins) / n
	m.AvgReturn = sumRet / n
	m.Expectancy = sumPL / n

	if m.GrossLoss > 0 {
		m.ProfitFactor = m.GrossProfit / m.GrossLoss
	} else if m.GrossProfit > 0 {
		m.ProfitFactor = math.Inf(1)
	}

	// Per-trade Sharpe (mean / stddev of returns).
	mean := m.AvgReturn
	var variance float64
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= n
	if sd := math.Sqrt(variance); sd > 0 {
		m.Sharpe = mean / sd * math.Sqrt(n)
	}

	// Max drawdown from the cumulative-PnL equity curve.
	var peak, equity float64
	for _, t := range trades {
		equity += t.RealizedPL
		if equity > peak {
			peak = equity
		}
		dd := peak - equity
		if dd > m.MaxDrawdown {
			m.MaxDrawdown = dd
			if peak > 0 {
				m.MaxDrawdownPct = dd / peak
			}
		}
	}
	return m
}

// String renders metrics as a human-readable report.
func (m Metrics) String() string {
	pf := fmt.Sprintf("%.2f", m.ProfitFactor)
	if math.IsInf(m.ProfitFactor, 1) {
		pf = "inf"
	}
	return fmt.Sprintf(
		"trades=%d win_rate=%.1f%% net_pl=$%.2f profit_factor=%s avg_return=%.2f%% expectancy=$%.4f sharpe=%.2f max_dd=$%.2f (%.1f%%)",
		m.Trades, m.WinRate*100, m.NetPL, pf, m.AvgReturn*100, m.Expectancy, m.Sharpe, m.MaxDrawdown, m.MaxDrawdownPct*100,
	)
}
