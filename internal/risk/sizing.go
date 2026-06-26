package risk

import (
	"math"

	"github.com/valsalamakha/livepolybtc/internal/config"
)

// Sizer computes the dollar notional to stake on a signal according to the
// configured sizing mode. The premium paid (notional) is also the maximum loss
// for this fade strategy, since a losing binary position expires worthless.
type Sizer struct {
	cfg config.SizingConfig
}

// NewSizer constructs a Sizer.
func NewSizer(cfg config.SizingConfig) *Sizer { return &Sizer{cfg: cfg} }

// Notional returns the dollar amount to stake given the current bankroll and
// the entry price of the faded side. The result is clamped to the configured
// minimum and maximum trade sizes (and to zero when below the minimum).
func (s *Sizer) Notional(bankroll, entryPrice float64) float64 {
	var raw float64
	switch s.cfg.Mode {
	case config.SizePercent:
		raw = bankroll * s.cfg.PercentOfBankroll
	case config.SizeKelly:
		raw = bankroll * s.kellyFraction(entryPrice)
	default: // SizeFixed
		raw = s.cfg.FixedSize
	}

	if raw > s.cfg.MaxTradeSize {
		raw = s.cfg.MaxTradeSize
	}
	if raw < s.cfg.MinTradeSize {
		return 0
	}
	if raw > bankroll {
		raw = bankroll
	}
	return raw
}

// kellyFraction computes the (scaled) Kelly fraction for a binary bet bought
// at entryPrice with assumed win probability cfg.WinProbability.
//
// For a $1-payoff binary contract bought at price p, the net odds are
// b = (1-p)/p. The full-Kelly fraction is f* = (q_win*b - q_lose)/b. We then
// apply the configured fractional-Kelly multiplier and floor at zero.
func (s *Sizer) kellyFraction(entryPrice float64) float64 {
	p := s.cfg.WinProbability
	if p <= 0 || p >= 1 || entryPrice <= 0 || entryPrice >= 1 {
		return 0
	}
	b := (1 - entryPrice) / entryPrice
	if b <= 0 {
		return 0
	}
	f := (p*b - (1 - p)) / b
	f *= s.cfg.KellyFraction
	return math.Max(0, math.Min(1, f))
}
