package backtest

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// GenerateSynthetic produces a deterministic synthetic dataset of BTC 5-minute
// up/down markets. Roughly half the markets contain a late overreaction (a
// rapid price spike on one side while BTC stays flat) that the strategy should
// fade; the fade resolves profitably with a configurable hit rate so the
// dataset exercises wins and losses. It is used for the demo backtest and for
// tests.
func GenerateSynthetic(numMarkets int, seed int64) []Tick {
	rng := rand.New(rand.NewSource(seed))
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	const (
		marketDur  = 300 * time.Second
		tickEvery  = time.Second
		fadeHitPct = 0.65 // probability the fade wins on overreaction markets
	)

	var ticks []Tick
	for i := 0; i < numMarkets; i++ {
		cond := fmt.Sprintf("0xCOND%04d", i)
		upTok := fmt.Sprintf("UP%04d", i)
		downTok := fmt.Sprintf("DN%04d", i)
		start := base.Add(time.Duration(i) * (marketDur + time.Minute))
		end := start.Add(marketDur)

		overreaction := rng.Float64() < 0.5
		spikeUp := rng.Float64() < 0.5 // which side spikes
		fadeWins := rng.Float64() < fadeHitPct

		btc := 60000 + rng.Float64()*2000
		for t := start; !t.After(end); t = t.Add(tickEvery) {
			ttr := end.Sub(t)
			// BTC quiet random walk; tiny steps so late move stays small.
			btc += (rng.Float64() - 0.5) * 2.0

			upMid, downMid := 0.5, 0.5
			upBidSz, upAskSz := 500.0, 500.0
			downBidSz, downAskSz := 500.0, 500.0

			if overreaction && ttr <= 15*time.Second && ttr > 2*time.Second {
				// Ramp the spiking side from 0.85 to 0.97 over ~5s.
				prog := clamp01(float64(15*time.Second-ttr) / float64(5*time.Second))
				spikeMid := 0.85 + 0.12*prog
				if spikeMid > 0.97 {
					spikeMid = 0.97
				}
				if spikeUp {
					upMid = spikeMid
					downMid = 1 - spikeMid
					upBidSz, upAskSz = 5000, 80     // strong buy imbalance
					downBidSz, downAskSz = 60, 2500 // deep cheap asks to fill the fade
				} else {
					downMid = spikeMid
					upMid = 1 - spikeMid
					downBidSz, downAskSz = 5000, 80
					upBidSz, upAskSz = 60, 2500
				}
			}

			// At resolution, force mids to reflect the realized winner.
			if ttr <= tickEvery {
				upMid, downMid = resolveMids(overreaction, spikeUp, fadeWins, rng)
			}

			ticks = append(ticks, Tick{
				Time:        t,
				ConditionID: cond,
				EndTime:     end,
				UpToken:     upTok,
				DownToken:   downTok,
				BTCPrice:    btc,
				UpBook:      twoSided(upTok, t, upMid, upBidSz, upAskSz),
				DownBook:    twoSided(downTok, t, downMid, downBidSz, downAskSz),
			})
		}
	}
	return ticks
}

// resolveMids returns the (up, down) mids at resolution encoding the winner.
func resolveMids(overreaction, spikeUp, fadeWins bool, rng *rand.Rand) (up, down float64) {
	win, lose := 0.99, 0.01
	if !overreaction {
		if rng.Float64() < 0.5 {
			return win, lose
		}
		return lose, win
	}
	// Fade wins => the side opposite the spike wins.
	spikeWins := !fadeWins
	if spikeUp == spikeWins {
		return win, lose
	}
	return lose, win
}

func twoSided(token string, ts time.Time, mid, bidSz, askSz float64) *models.OrderBook {
	const halfSpread = 0.01
	bid := mid - halfSpread
	ask := mid + halfSpread
	if bid < 0.01 {
		bid = 0.01
	}
	if ask > 0.99 {
		ask = 0.99
	}
	ob := &models.OrderBook{
		TokenID:   token,
		Timestamp: ts,
		Bids:      []models.PriceLevel{{Price: round2(bid), Size: bidSz}},
		Asks:      []models.PriceLevel{{Price: round2(ask), Size: askSz}},
	}
	ob.Normalize()
	return ob
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

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
