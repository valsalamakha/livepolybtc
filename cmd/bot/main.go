// Command bot runs the Polymarket Bitcoin 5-minute up/down trading bot.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/valsalamakha/livepolybtc/internal/app"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
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
		dryRun     = flag.Bool("dry-run", false, "force simulation mode (no live orders)")
		live       = flag.Bool("live", false, "enable live trading (requires credentials)")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if *dryRun {
		cfg.DryRun = true
	}
	if *live {
		cfg.DryRun = false
	}

	log := logging.New(logging.ParseLevel(cfg.Logging.Level), cfg.Logging.Console)

	if !cfg.DryRun {
		if err := cfg.RequireCredentials(); err != nil {
			return fmt.Errorf("live trading: %w", err)
		}
		log.Warnf("LIVE TRADING ENABLED — orders may be submitted to Polymarket")
	}

	rec, err := logging.NewEventRecorder(cfg.Logging.Dir, cfg.Logging.CSV, cfg.Logging.JSON)
	if err != nil {
		return fmt.Errorf("init event recorder: %w", err)
	}
	defer rec.Close()

	bot, err := app.New(cfg, log, rec)
	if err != nil {
		return fmt.Errorf("init bot: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return bot.Run(ctx)
}
