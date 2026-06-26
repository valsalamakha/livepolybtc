package risk

import (
	"testing"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

func baseCfg() (config.RiskConfig, config.SizingConfig) {
	c := config.Default()
	return c.Risk, c.Sizing
}

func TestSizerFixed(t *testing.T) {
	s := NewSizer(config.SizingConfig{Mode: config.SizeFixed, FixedSize: 20, MaxTradeSize: 50, MinTradeSize: 1})
	if got := s.Notional(1000, 0.05); got != 20 {
		t.Errorf("fixed notional = %v, want 20", got)
	}
}

func TestSizerPercent(t *testing.T) {
	s := NewSizer(config.SizingConfig{Mode: config.SizePercent, PercentOfBankroll: 0.02, MaxTradeSize: 100, MinTradeSize: 1})
	if got := s.Notional(1000, 0.05); got != 20 {
		t.Errorf("percent notional = %v, want 20", got)
	}
}

func TestSizerMaxCap(t *testing.T) {
	s := NewSizer(config.SizingConfig{Mode: config.SizeFixed, FixedSize: 200, MaxTradeSize: 50, MinTradeSize: 1})
	if got := s.Notional(1000, 0.05); got != 50 {
		t.Errorf("capped notional = %v, want 50", got)
	}
}

func TestSizerKellyPositive(t *testing.T) {
	// p=0.6 at price 0.05 => strong edge => positive stake.
	s := NewSizer(config.SizingConfig{Mode: config.SizeKelly, KellyFraction: 0.5, WinProbability: 0.6, MaxTradeSize: 1000, MinTradeSize: 1})
	if got := s.Notional(1000, 0.05); got <= 0 {
		t.Errorf("kelly notional = %v, want > 0", got)
	}
}

func TestCanTradeMaxPositions(t *testing.T) {
	rc, sc := baseCfg()
	rc.MaxSimultaneousPositions = 1
	m := NewManager(rc, sc)
	m.OnEntry(&models.Position{ID: "p1", Size: 100, EntryPrice: 0.05})
	if ok, _ := m.CanTrade(); ok {
		t.Error("expected CanTrade false with max positions open")
	}
}

func TestConsecutiveLossesHalt(t *testing.T) {
	rc, sc := baseCfg()
	rc.MaxConsecutiveLosses = 2
	rc.DailyStopLoss = 0 // disable to isolate
	m := NewManager(rc, sc)
	for i := 0; i < 2; i++ {
		id := "p" + string(rune('a'+i))
		m.OnEntry(&models.Position{ID: id, Size: 100, EntryPrice: 0.05})
		m.OnSettle(id, 0, -5) // loss
	}
	if ok, reason := m.CanTrade(); ok {
		t.Errorf("expected halt after consecutive losses, got ok (reason=%q)", reason)
	}
}

func TestWinResetsConsecutiveLosses(t *testing.T) {
	rc, sc := baseCfg()
	rc.MaxConsecutiveLosses = 2
	rc.DailyStopLoss = 0
	m := NewManager(rc, sc)
	m.OnEntry(&models.Position{ID: "a", Size: 100, EntryPrice: 0.05})
	m.OnSettle("a", 0, -5)
	m.OnEntry(&models.Position{ID: "b", Size: 100, EntryPrice: 0.05})
	m.OnSettle("b", 100, 95) // win
	if m.State().ConsecutiveLosses != 0 {
		t.Errorf("consecutive losses = %d, want 0 after a win", m.State().ConsecutiveLosses)
	}
}

func TestDailyStopLossTripsKill(t *testing.T) {
	rc, sc := baseCfg()
	rc.DailyStopLoss = 10
	m := NewManager(rc, sc)
	m.OnEntry(&models.Position{ID: "a", Size: 100, EntryPrice: 0.2})
	m.OnSettle("a", 0, -20) // exceeds daily stop
	if !m.Killed() {
		t.Error("expected kill switch to trip on daily stop loss")
	}
	if ok, _ := m.CanTrade(); ok {
		t.Error("expected CanTrade false after kill")
	}
}

func TestExposureLimits(t *testing.T) {
	rc, sc := baseCfg()
	rc.MaxExposure = 30
	rc.MaxSimultaneousPositions = 10
	m := NewManager(rc, sc)
	m.OnEntry(&models.Position{ID: "a", Size: 100, EntryPrice: 0.2}) // exposure 20
	if err := m.CheckEntry(20); err == nil {
		t.Error("expected exposure check to fail (20 + 20 > 30)")
	}
	if err := m.CheckEntry(5); err != nil {
		t.Errorf("expected exposure check to pass for 5: %v", err)
	}
}

func TestBankrollAccounting(t *testing.T) {
	rc, sc := baseCfg()
	sc.Bankroll = 1000
	m := NewManager(rc, sc)
	pos := &models.Position{ID: "a", Size: 100, EntryPrice: 0.05} // cost 5
	m.OnEntry(pos)
	if got := m.State().Bankroll; got != 995 {
		t.Errorf("bankroll after entry = %v, want 995", got)
	}
	m.OnSettle("a", 100, 95) // win: payoff 100 returns
	if got := m.State().Bankroll; got != 1095 {
		t.Errorf("bankroll after win = %v, want 1095", got)
	}
}

func TestKillAndReset(t *testing.T) {
	rc, sc := baseCfg()
	m := NewManager(rc, sc)
	m.Kill("manual")
	if !m.Killed() {
		t.Fatal("expected killed")
	}
	m.Reset()
	if m.Killed() {
		t.Error("expected reset to clear kill switch")
	}
}

func TestDayRollover(t *testing.T) {
	rc, sc := baseCfg()
	rc.DailyStopLoss = 0
	m := NewManager(rc, sc)
	now := time.Now().UTC()
	m.clock = func() time.Time { return now }
	m.OnEntry(&models.Position{ID: "a", Size: 100, EntryPrice: 0.05})
	m.OnSettle("a", 0, -5)
	// Advance one day.
	m.clock = func() time.Time { return now.Add(25 * time.Hour) }
	if got := m.State().DailyTrades; got != 0 {
		t.Errorf("daily trades after rollover = %d, want 0", got)
	}
}
