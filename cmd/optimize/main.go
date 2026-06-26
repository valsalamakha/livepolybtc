// Command optimize grid-searches strategy parameters over a dataset and prints
// the best-performing combinations.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/backtest"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/optimize"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath = flag.String("config", "configs/config.yaml", "path to YAML configuration file")
		dataPath   = flag.String("data", "", "path to a CSV dataset (omit to use synthetic data)")
		synthN     = flag.Int("synthetic", 300, "number of synthetic markets when no data is given")
		seed       = flag.Int64("seed", 42, "synthetic data seed")
		objective  = flag.String("objective", "netpl", "ranking objective: netpl|sharpe|expectancy")
		top        = flag.Int("top", 10, "number of top results to print")
		jsonOut    = flag.Bool("json", false, "emit results as JSON")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var ticks []backtest.Tick
	if *dataPath != "" {
		ticks, err = backtest.LoadCSV(*dataPath)
		if err != nil {
			return fmt.Errorf("load data: %w", err)
		}
	} else {
		ticks = backtest.GenerateSynthetic(*synthN, *seed)
	}
	fmt.Printf("optimizing over %d ticks...\n", len(ticks))

	grid := optimize.Grid{
		MaximumBTCMove:            []float64{10, 15, 25, 40},
		MinimumPriceSpike:         []float64{0.06, 0.08, 0.10},
		FinalObservationWindow:    []time.Duration{15 * time.Second, 20 * time.Second, 30 * time.Second},
		MinimumOrderBookImbalance: []float64{0.3, 0.5, 0.7},
		MinimumExpectedReturn:     []float64{4.0, 4.5, 6.0},
	}

	obj := objectiveFor(*objective)
	results := optimize.Run(*cfg, ticks, grid, obj)

	if *jsonOut {
		n := *top
		if n > len(results) {
			n = len(results)
		}
		b, _ := json.MarshalIndent(results[:n], "", "  ")
		fmt.Println(string(b))
		return nil
	}

	fmt.Printf("\n=== Top %d combinations by %s (of %d tested) ===\n", *top, *objective, len(results))
	for i, r := range results {
		if i >= *top {
			break
		}
		fmt.Printf("#%d score=%.4f | btcMove=%.0f spike=%.2f window=%s imb=%.2f expRet=%.1f\n",
			i+1, r.Score, r.Params.MaximumBTCMove, r.Params.MinimumPriceSpike,
			r.Params.FinalObservationWindow, r.Params.MinimumOrderBookImbalance, r.Params.MinimumExpectedReturn)
		fmt.Printf("     %s\n", r.Metrics.String())
	}
	return nil
}

func objectiveFor(name string) optimize.Objective {
	switch name {
	case "sharpe":
		return optimize.SharpeObjective
	case "expectancy":
		return optimize.ExpectancyObjective
	default:
		return optimize.NetPLObjective
	}
}
