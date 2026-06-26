package app

import (
	"context"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/models"
	"github.com/valsalamakha/livepolybtc/internal/strategy"
)

// marketLifecycle continuously discovers the active market, drives collection
// for its lifetime, and rolls over to the next market on resolution.
func (b *Bot) marketLifecycle(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		market, err := b.disc.Discover(ctx)
		if err != nil {
			b.log.Warnf("market discovery: %v", err)
			if !sleepCtx(ctx, b.cfg.Market.DiscoveryInterval) {
				return
			}
			continue
		}

		b.coll.SetMarket(market)
		ttr := time.Until(market.EndTime)
		b.log.Infof("trading market %q (condition %s); resolves in %s",
			market.Question, short(market.ConditionID), ttr.Round(time.Second))

		// Bound collection to just past resolution, then rediscover.
		deadline := market.EndTime.Add(3 * time.Second)
		if market.EndTime.IsZero() {
			deadline = time.Now().Add(b.cfg.Market.DiscoveryInterval)
		}
		mctx, cancel := context.WithDeadline(ctx, deadline)
		b.coll.Run(mctx) // blocks until deadline or shutdown
		cancel()
	}
}

// strategyLoop evaluates snapshots on a fast cadence (sub-100ms) and forwards
// qualifying signals to the execution loop.
func (b *Bot) strategyLoop(ctx context.Context) {
	const tick = 50 * time.Millisecond
	t := time.NewTicker(tick)
	defer t.Stop()

	var lastCondition string
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			market := b.coll.Market()
			if market == nil {
				continue
			}
			// Reset per-market strategy state on rollover.
			if market.ConditionID != lastCondition {
				b.engine = strategy.NewEngine(b.cfg.Strategy)
				lastCondition = market.ConditionID
			}

			snap := b.coll.Snapshot(b.feed.Stats())
			if snap.Market == nil {
				continue
			}
			sig, ok := b.engine.Evaluate(snap)
			if !ok {
				continue
			}
			b.recordSignal(sig, snap)
			if allowed, reason := b.risk.CanTrade(); !allowed {
				b.log.Infof("signal suppressed by risk: %s", reason)
				continue
			}
			select {
			case b.signals <- sig:
			default:
				b.log.Warnf("signal dropped: execution queue full")
			}
		}
	}
}

// executionLoop consumes signals, sizes them, applies the final risk gate, and
// executes them.
func (b *Bot) executionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-b.signals:
			b.handleSignal(ctx, sig)
		}
	}
}

func (b *Bot) handleSignal(ctx context.Context, sig *models.Signal) {
	notional := b.risk.SizeFor(sig)
	if notional <= 0 {
		b.log.Infof("skip signal: sizing returned zero notional")
		return
	}
	if err := b.risk.CheckEntry(notional); err != nil {
		b.log.Infof("skip signal: %v", err)
		return
	}

	sizeShares := notional / sig.EntryPrice
	snap := b.coll.Snapshot(b.feed.Stats())
	entryBook := snap.Book(sig.EntryTokenID)

	res, err := b.exec.Execute(ctx, sig, sizeShares, entryBook)
	if err != nil {
		b.log.Errorf("execution failed: %v", err)
		b.recordOrderError(sig, err)
		return
	}
	b.recordFill(sig, res)

	if res.FilledSize <= 0 {
		return
	}

	market := b.coll.Market()
	pos := &models.Position{
		ID:          res.Order.ClientID,
		ConditionID: sig.ConditionID,
		TokenID:     sig.EntryTokenID,
		Outcome:     sig.EntryOutcome,
		Size:        res.FilledSize,
		EntryPrice:  res.AvgFillPrice,
		EntryTime:   time.Now(),
	}
	b.risk.OnEntry(pos)

	endTime := time.Now().Add(b.cfg.Strategy.ExitDeadline)
	if market != nil && !market.EndTime.IsZero() {
		endTime = market.EndTime
	}
	b.mu.Lock()
	b.positions[pos.ID] = &trackedPosition{pos: pos, endTime: endTime}
	b.mu.Unlock()

	b.log.Infof("ENTER %s %.2f shares @ %.4f ($%.2f) — %s",
		sig.EntryOutcome, res.FilledSize, res.AvgFillPrice, res.Notional(), sig.Reason)
}

// settlementLoop resolves positions whose markets have ended and updates risk
// accounting.
func (b *Bot) settlementLoop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.settlePending(ctx)
		}
	}
}

func (b *Bot) settlePending(ctx context.Context) {
	now := time.Now()
	b.mu.Lock()
	var due []*trackedPosition
	for _, tp := range b.positions {
		if !tp.settled && now.After(tp.endTime.Add(2*time.Second)) {
			due = append(due, tp)
		}
	}
	b.mu.Unlock()

	for _, tp := range due {
		won, ok := b.resolveOutcome(ctx, tp.pos.TokenID)
		if !ok {
			continue // try again next tick
		}
		payoff := 0.0
		if won {
			payoff = tp.pos.MaxPayoff()
		}
		realized := payoff - tp.pos.Cost()

		tp.pos.Closed = true
		tp.pos.ExitTime = now
		tp.pos.RealizedPL = realized
		if won {
			tp.pos.ExitPrice = 1.0
		}
		b.risk.OnSettle(tp.pos.ID, payoff, realized)

		b.mu.Lock()
		tp.settled = true
		b.mu.Unlock()

		result := "LOSS"
		if won {
			result = "WIN"
		}
		b.log.Infof("SETTLE %s %s: realized $%.2f (bankroll $%.2f)",
			tp.pos.Outcome, result, realized, b.risk.State().Bankroll)
		b.recordSettle(tp.pos, realized)
	}
}

// resolveOutcome determines whether the position's token resolved in the money
// by reading its post-resolution midpoint. The boolean is false when the
// outcome cannot yet be determined.
func (b *Bot) resolveOutcome(ctx context.Context, tokenID string) (won bool, ok bool) {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	mid, err := b.client.GetMidpoint(cctx, tokenID)
	if err != nil {
		// Fall back to the last known book mid from the collector.
		snap := b.coll.Snapshot(b.feed.Stats())
		if ob := snap.Book(tokenID); ob != nil {
			mid = ob.MidPrice()
		} else {
			return false, false
		}
	}
	if mid <= 0 {
		return false, false
	}
	return mid >= 0.5, true
}

// healthLoop emits heartbeats and watches feed/connection health.
func (b *Bot) healthLoop(ctx context.Context) {
	t := time.NewTicker(b.cfg.Health.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			connected, since := b.coll.Health()
			btc := b.feed.Stats()
			state := b.risk.State()
			b.log.Infof("heartbeat: bankroll=$%.2f dailyPL=$%.2f open=%d ws=%v btc=$%.0f stale=%v",
				state.Bankroll, state.DailyPL, state.OpenPositions, connected, btc.Price, btc.Stale)
			b.rec.Record(logging.Event{
				Type:     logging.EventHeartbeat,
				BTCPrice: btc.Price,
				Profit:   state.DailyPL,
				Message:  heartbeatMsg(connected, since, btc.Stale, state.Killed),
			})
			if btc.Stale {
				b.log.Warnf("BTC feed is stale")
			}
			if b.cfg.BTCFeed.UseWebsocket && !connected && since > 30*time.Second {
				b.log.Warnf("market websocket disconnected for %s", since.Round(time.Second))
			}
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		d = time.Second
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + ".." + s[len(s)-4:]
}
