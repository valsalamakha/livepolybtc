// Package risk implements the bot's risk-control framework: position sizing,
// pre-trade gating, exposure and loss limits, and a kill switch. The Manager
// is the single source of truth for tradeability and is safe for concurrent
// use.
package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Manager enforces all risk controls and tracks live account state.
type Manager struct {
	cfg   config.RiskConfig
	sizer *Sizer

	mu                sync.Mutex
	bankroll          float64
	startBankroll     float64
	dailyPL           float64
	consecutiveLosses int
	dailyTrades       int
	dayStart          time.Time

	openPositions map[string]*models.Position
	exposure      float64

	killed     bool
	killReason string

	clock func() time.Time
}

// NewManager constructs a risk Manager seeded with the starting bankroll.
func NewManager(riskCfg config.RiskConfig, sizingCfg config.SizingConfig) *Manager {
	now := time.Now()
	return &Manager{
		cfg:           riskCfg,
		sizer:         NewSizer(sizingCfg),
		bankroll:      sizingCfg.Bankroll,
		startBankroll: sizingCfg.Bankroll,
		dayStart:      truncateToDay(now),
		openPositions: make(map[string]*models.Position),
		clock:         time.Now,
	}
}

// CanTrade reports whether new entries are currently permitted, and a reason
// when they are not.
func (m *Manager) CanTrade() (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollDayLocked()

	if m.cfg.KillSwitchEnabled && m.killed {
		return false, "kill switch active: " + m.killReason
	}
	if m.cfg.DailyStopLoss > 0 && m.dailyPL <= -m.cfg.DailyStopLoss {
		return false, fmt.Sprintf("daily stop loss hit (%.2f <= -%.2f)", m.dailyPL, m.cfg.DailyStopLoss)
	}
	if m.cfg.MaxConsecutiveLosses > 0 && m.consecutiveLosses >= m.cfg.MaxConsecutiveLosses {
		return false, fmt.Sprintf("max consecutive losses reached (%d)", m.consecutiveLosses)
	}
	if m.cfg.MaxDailyTrades > 0 && m.dailyTrades >= m.cfg.MaxDailyTrades {
		return false, fmt.Sprintf("max daily trades reached (%d)", m.dailyTrades)
	}
	if len(m.openPositions) >= m.cfg.MaxSimultaneousPositions {
		return false, fmt.Sprintf("max simultaneous positions open (%d)", len(m.openPositions))
	}
	return true, ""
}

// SizeFor computes the dollar notional to stake on a signal, after applying
// the per-trade loss cap and remaining-exposure headroom. A zero result means
// the trade should be skipped.
func (m *Manager) SizeFor(sig *models.Signal) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	notional := m.sizer.Notional(m.bankroll, sig.EntryPrice)
	if notional <= 0 {
		return 0
	}
	// Max loss for this fade == premium == notional.
	if m.cfg.MaxLossPerTrade > 0 && notional > m.cfg.MaxLossPerTrade {
		notional = m.cfg.MaxLossPerTrade
	}
	// Respect total-exposure headroom.
	if m.cfg.MaxExposure > 0 {
		room := m.cfg.MaxExposure - m.exposure
		if room <= 0 {
			return 0
		}
		if notional > room {
			notional = room
		}
	}
	return notional
}

// CheckEntry is the final gate before submitting an order for the given
// notional. It re-validates tradeability and exposure.
func (m *Manager) CheckEntry(notional float64) error {
	if notional <= 0 {
		return fmt.Errorf("non-positive notional")
	}
	if ok, reason := m.CanTrade(); !ok {
		return fmt.Errorf("trading blocked: %s", reason)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.MaxExposure > 0 && m.exposure+notional > m.cfg.MaxExposure {
		return fmt.Errorf("would exceed max exposure (%.2f + %.2f > %.2f)", m.exposure, notional, m.cfg.MaxExposure)
	}
	if notional > m.bankroll {
		return fmt.Errorf("insufficient bankroll (%.2f > %.2f)", notional, m.bankroll)
	}
	return nil
}

// OnEntry records a newly opened position: it debits the premium from the
// bankroll, adds exposure, and increments the daily trade counter.
func (m *Manager) OnEntry(pos *models.Position) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollDayLocked()
	m.openPositions[pos.ID] = pos
	cost := pos.Cost()
	m.bankroll -= cost
	m.exposure += cost
	m.dailyTrades++
}

// OnSettle records the resolution of a position. realizedPL is the net profit
// (payoff minus premium). It updates bankroll, daily PnL, consecutive-loss
// tracking, and exposure, and auto-trips the kill switch on the daily stop.
func (m *Manager) OnSettle(posID string, payoff, realizedPL float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollDayLocked()

	pos, ok := m.openPositions[posID]
	if ok {
		m.exposure -= pos.Cost()
		if m.exposure < 0 {
			m.exposure = 0
		}
		delete(m.openPositions, posID)
	}
	// Payoff (winnings on settlement) returns to bankroll; premium was already
	// debited at entry.
	m.bankroll += payoff
	m.dailyPL += realizedPL

	if realizedPL < 0 {
		m.consecutiveLosses++
	} else if realizedPL > 0 {
		m.consecutiveLosses = 0
	}

	if m.cfg.DailyStopLoss > 0 && m.dailyPL <= -m.cfg.DailyStopLoss {
		m.tripKillLocked("daily stop loss reached")
	}
}

// Kill manually trips the kill switch.
func (m *Manager) Kill(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tripKillLocked(reason)
}

func (m *Manager) tripKillLocked(reason string) {
	if !m.killed {
		m.killed = true
		m.killReason = reason
	}
}

// Reset clears the kill switch (operator override).
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.killed = false
	m.killReason = ""
	m.consecutiveLosses = 0
}

// Killed reports whether the kill switch is tripped.
func (m *Manager) Killed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.killed
}

// State is a point-in-time snapshot of risk state for reporting.
type State struct {
	Bankroll          float64
	DailyPL           float64
	ConsecutiveLosses int
	DailyTrades       int
	OpenPositions     int
	Exposure          float64
	Killed            bool
	KillReason        string
}

// State returns a snapshot of the current risk state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollDayLocked()
	return State{
		Bankroll:          m.bankroll,
		DailyPL:           m.dailyPL,
		ConsecutiveLosses: m.consecutiveLosses,
		DailyTrades:       m.dailyTrades,
		OpenPositions:     len(m.openPositions),
		Exposure:          m.exposure,
		Killed:            m.killed,
		KillReason:        m.killReason,
	}
}

// OpenPositions returns a copy of the currently open positions.
func (m *Manager) OpenPositions() []*models.Position {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*models.Position, 0, len(m.openPositions))
	for _, p := range m.openPositions {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

// rollDayLocked resets daily counters when the UTC day changes. Caller holds m.mu.
func (m *Manager) rollDayLocked() {
	today := truncateToDay(m.clock())
	if today.After(m.dayStart) {
		m.dayStart = today
		m.dailyPL = 0
		m.dailyTrades = 0
	}
}

func truncateToDay(t time.Time) time.Time {
	y, mo, d := t.UTC().Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
}
