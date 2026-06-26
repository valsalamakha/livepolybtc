// Command backtest replays historical (or synthetic) market data through the
// strategy and reports performance metrics.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/valsalamakha/livepolybtc/internal/backtest"
	"github.com/valsalamakha/livepolybtc/internal/config"
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
		dataPath   = flag.String("data", "", "path to a CSV dataset (omit to use a synthetic dataset)")
		synthN     = flag.Int("synthetic", 200, "number of synthetic markets to generate when no data is given")
		seed       = flag.Int64("seed", 42, "synthetic data seed")
		jsonOut    = flag.Bool("json", false, "emit metrics as JSON")
		showTrades = flag.Bool("trades", false, "print each trade")
		export     = flag.String("export", "", "write the dataset to this CSV path and exit")
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
		fmt.Printf("loaded %d ticks from %s\n", len(ticks), *dataPath)
	} else {
		ticks = backtest.GenerateSynthetic(*synthN, *seed)
		fmt.Printf("generated %d synthetic ticks across %d markets\n", len(ticks), *synthN)
	}

	if *export != "" {
		if err := backtest.WriteCSV(*export, ticks); err != nil {
			return fmt.Errorf("export: %w", err)
		}
		fmt.Printf("wrote %d ticks to %s\n", len(ticks), *export)
		return nil
	}

	res := backtest.New(*cfg).Run(ticks)

	if *showTrades {
		for _, t := range res.Trades {
			result := "LOSS"
			if t.Won {
				result = "WIN "
			}
			fmt.Printf("  %s %-4s entry=%.3f size=%.1f pl=$%+.3f  %s\n",
				result, t.Outcome, t.EntryPrice, t.Size, t.RealizedPL, t.ConditionID)
		}
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(res.Metrics, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	fmt.Println("\n=== Backtest Results ===")
	fmt.Println(res.Metrics.String())
	return nil
}
