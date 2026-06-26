// Package app wires the bot's components together and runs them as
// coordinated goroutines communicating over channels, with graceful shutdown
// driven by context cancellation.
package app

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/auth"
	"github.com/valsalamakha/livepolybtc/internal/btcfeed"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/execution"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/marketdata"
	"github.com/valsalamakha/livepolybtc/internal/models"
	"github.com/valsalamakha/livepolybtc/internal/risk"
	"github.com/valsalamakha/livepolybtc/internal/strategy"
)

// Bot is the top-level application orchestrator.
type Bot struct {
	cfg    *config.Config
	log    *logging.Logger
	rec    *logging.EventRecorder
	client *api.Client
	feed   *btcfeed.Feed
	disc   *marketdata.Discoverer
	coll   *marketdata.Collector
	engine *strategy.Engine
	risk   *risk.Manager
	exec   *execution.Executor

	signals chan *models.Signal

	mu        sync.Mutex
	positions map[string]*trackedPosition
}

// trackedPosition couples a position with the market metadata needed to settle
// it after resolution.
type trackedPosition struct {
	pos     *models.Position
	endTime time.Time
	settled bool
}

// New constructs a fully wired Bot from configuration.
func New(cfg *config.Config, log *logging.Logger, rec *logging.EventRecorder) (*Bot, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	signer := auth.NewSigner(auth.Credentials{
		APIKey:     cfg.Polymarket.APIKey,
		Secret:     cfg.Polymarket.Secret,
		Passphrase: cfg.Polymarket.Passphrase,
		Address:    cfg.Polymarket.Address,
	})

	client := api.New(api.Options{
		CLOBBaseURL:  cfg.Polymarket.RestBaseURL,
		GammaBaseURL: cfg.Polymarket.GammaBaseURL,
		ChainID:      cfg.Polymarket.ChainID,
		Signer:       signer,
		HTTPClient:   httpClient,
		MaxRetries:   cfg.Execution.MaxRetries,
		RetryBackoff: cfg.Execution.RetryBackoff,
	})

	feed, err := btcfeed.New(cfg.BTCFeed, httpClient)
	if err != nil {
		return nil, err
	}

	disc := marketdata.NewDiscoverer(client, cfg.Market)
	coll := marketdata.NewCollector(client, log, cfg.Polymarket, cfg.Health, cfg.BTCFeed.UseWebsocket)
	riskMgr := risk.NewManager(cfg.Risk, cfg.Sizing)

	orderSigner := execution.NewUnsignedSigner(cfg.Polymarket.Address)
	exec := execution.NewExecutor(client, orderSigner, log, cfg.Execution, cfg.DryRun)

	return &Bot{
		cfg:       cfg,
		log:       log,
		rec:       rec,
		client:    client,
		feed:      feed,
		disc:      disc,
		coll:      coll,
		engine:    strategy.NewEngine(cfg.Strategy),
		risk:      riskMgr,
		exec:      exec,
		signals:   make(chan *models.Signal, 16),
		positions: make(map[string]*trackedPosition),
	}, nil
}

// Run starts every component goroutine and blocks until ctx is cancelled, then
// performs graceful shutdown.
func (b *Bot) Run(ctx context.Context) error {
	mode := "LIVE"
	if b.exec.Simulated() {
		mode = "SIMULATED/PAPER"
	}
	b.log.Infof("starting bot: btc=%s mode=%s dry_run=%v", b.feed.Exchange(), mode, b.cfg.DryRun)

	var wg sync.WaitGroup
	run := func(name string, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer b.recover(name)
			fn()
		}()
	}

	run("btc-feed", func() { _ = b.feed.Run(ctx) })
	run("market-lifecycle", func() { b.marketLifecycle(ctx) })
	run("strategy", func() { b.strategyLoop(ctx) })
	run("execution", func() { b.executionLoop(ctx) })
	run("settlement", func() { b.settlementLoop(ctx) })
	run("health", func() { b.healthLoop(ctx) })

	<-ctx.Done()
	b.log.Infof("shutdown requested; draining...")
	b.shutdown()
	wg.Wait()
	b.log.Infof("bot stopped")
	return nil
}

func (b *Bot) recover(name string) {
	if r := recover(); r != nil {
		b.log.Errorf("goroutine %q panicked: %v", name, r)
		b.risk.Kill("panic in " + name)
	}
}

// shutdown performs best-effort cleanup: cancel resting orders and emergency
// liquidation when configured.
func (b *Bot) shutdown() {
	cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !b.exec.Simulated() {
		if err := b.client.CancelAll(cctx); err != nil {
			b.log.Warnf("cancel-all on shutdown: %v", err)
		}
	}
}
