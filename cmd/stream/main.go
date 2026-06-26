// Command stream is a Stage-1 diagnostic: it discovers the live Bitcoin
// 5-minute up/down market and prints the real-time market data stream (order
// books, best bid/ask, spread, implied probability, imbalance, liquidity, last
// trade, volume) alongside the live BTC price — to prove the data feed is
// correct before any trading logic is exercised.
//
// Market data is PUBLIC and needs no credentials. Use -check-auth to separately
// verify your API key/secret/passphrase by calling an authenticated endpoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/auth"
	"github.com/valsalamakha/livepolybtc/internal/btcfeed"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/marketdata"
	"github.com/valsalamakha/livepolybtc/internal/models"
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
		interval   = flag.Duration("interval", time.Second, "how often to print a snapshot")
		duration   = flag.Duration("duration", 0, "auto-stop after this long (0 = run until Ctrl-C)")
		raw        = flag.Bool("raw", false, "also dump raw websocket frames as they arrive")
		once       = flag.Bool("once", false, "fetch a single REST snapshot and exit (no websocket)")
		noWS       = flag.Bool("no-ws", false, "disable websocket; use REST polling only")
		checkAuth  = flag.Bool("check-auth", false, "verify credentials via an authenticated balance call")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logging.New(logging.ParseLevel(cfg.Logging.Level), false)

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
		MaxRetries:   2,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}

	// Optional: prove the API login works (authenticated endpoint).
	if *checkAuth {
		if err := cfg.RequireCredentials(); err != nil {
			return fmt.Errorf("check-auth: %w", err)
		}
		fmt.Println("== Auth check: calling authenticated /balance-allowance ==")
		bal, err := client.GetBalance(ctx)
		if err != nil {
			return fmt.Errorf("auth check FAILED (credentials rejected or endpoint error): %w", err)
		}
		fmt.Printf("auth OK — collateral balance=%s allowance=%s\n\n", bal.Balance, bal.Allowance)
	}

	// Discover the active market (public).
	disc := marketdata.NewDiscoverer(client, cfg.Market)
	fmt.Printf("== Discovering active market (keywords=%v) ==\n", cfg.Market.Keywords)
	market, err := disc.Discover(ctx)
	if err != nil {
		return fmt.Errorf("discovery FAILED: %w", err)
	}
	printMarket(market)

	if *once {
		return printOnce(ctx, client, cfg, market)
	}

	// Live BTC feed.
	feed, err := btcfeed.New(cfg.BTCFeed, nil)
	if err != nil {
		return err
	}
	go func() { _ = feed.Run(ctx) }()

	useWS := cfg.BTCFeed.UseWebsocket && !*noWS
	coll := marketdata.NewCollector(client, log, cfg.Polymarket, cfg.Health, useWS)
	if *raw {
		coll.RawHandler = func(b []byte) {
			fmt.Printf("  RAW %s\n", compact(b))
		}
	}

	fmt.Printf("== Streaming (%s, ws=%v) — press Ctrl-C to stop ==\n\n", *interval, useWS)
	return streamLoop(ctx, coll, feed, disc, cfg, market, *interval)
}

// streamLoop runs the collector for each market's lifetime, printing snapshots,
// and rolls over to the next market when one resolves.
func streamLoop(ctx context.Context, coll *marketdata.Collector, feed *btcfeed.Feed, disc *marketdata.Discoverer, cfg *config.Config, market *models.Market, interval time.Duration) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		coll.SetMarket(market)

		deadline := market.EndTime.Add(3 * time.Second)
		if market.EndTime.IsZero() {
			deadline = time.Now().Add(cfg.Market.DiscoveryInterval)
		}
		mctx, cancel := context.WithDeadline(ctx, deadline)
		go coll.Run(mctx)

		t := time.NewTicker(interval)
	inner:
		for {
			select {
			case <-mctx.Done():
				t.Stop()
				cancel()
				break inner
			case <-t.C:
				printSnapshot(coll.Snapshot(feed.Stats()), coll)
			}
		}
		if ctx.Err() != nil {
			return nil
		}

		// Roll over to the next market.
		fmt.Println("\n-- market resolved; rediscovering --")
		next, err := disc.Discover(ctx)
		if err != nil {
			fmt.Printf("rediscovery failed: %v (retrying)\n", err)
			if !sleepCtx(ctx, cfg.Market.DiscoveryInterval) {
				return nil
			}
			continue
		}
		market = next
		printMarket(market)
	}
}

func printOnce(ctx context.Context, client *api.Client, cfg *config.Config, market *models.Market) error {
	fmt.Println("\n== Single REST snapshot ==")
	for _, tok := range market.Tokens {
		ob, err := client.GetOrderBook(ctx, tok.TokenID)
		if err != nil {
			return fmt.Errorf("get book %s (%s): %w", tok.Outcome, tok.TokenID, err)
		}
		printBookLine(tok.Outcome, ob)
	}
	src, err := btcfeed.NewSource(cfg.BTCFeed.Exchange, cfg.BTCFeed.Symbol, nil)
	if err != nil {
		return err
	}
	price, err := src.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("btc fetch (%s): %w", cfg.BTCFeed.Exchange, err)
	}
	fmt.Printf("  BTC  (%s) $%.2f\n", src.Name(), price)
	return nil
}

func printMarket(m *models.Market) {
	fmt.Printf("\nMarket : %s\n", m.Question)
	fmt.Printf("Slug   : %s\n", m.Slug)
	fmt.Printf("Cond   : %s\n", m.ConditionID)
	fmt.Printf("Ends   : %s (in %s)\n", m.EndTime.Format(time.RFC3339), time.Until(m.EndTime).Round(time.Second))
	for _, t := range m.Tokens {
		fmt.Printf("Token  : %-5s %s\n", t.Outcome, t.TokenID)
	}
	fmt.Println()
}

func printSnapshot(snap models.Snapshot, coll *marketdata.Collector) {
	connected, since := coll.Health()
	wsMark := "✗"
	if connected {
		wsMark = "✓"
	}
	ttr := snap.TimeToResolution.Round(time.Second)
	b := snap.BTC
	fmt.Printf("─ %s ─ ttr %s ─ ws:%s (last %s ago) ─\n",
		snap.Timestamp.Format("15:04:05.000"), ttr, wsMark, since.Round(time.Millisecond))
	fmt.Printf("  BTC  $%.2f  Δ5s $%.2f  Δ20s $%.2f  vel $%.2f/s  vol5s %.2f  stale:%v\n",
		b.Price, b.Move5s, b.Move20s, b.Velocity, b.Volatility5s, b.Stale)
	if snap.Market != nil {
		for _, tok := range snap.Market.Tokens {
			ob := snap.Book(tok.TokenID)
			lt := snap.LastTrades[tok.TokenID]
			vol := snap.Volume[tok.TokenID]
			printBookLineFull(tok.Outcome, ob, lt, vol)
		}
	}
	fmt.Println()
}

func printBookLine(outcome string, ob *models.OrderBook) {
	printBookLineFull(outcome, ob, models.Trade{}, 0)
}

func printBookLineFull(outcome string, ob *models.OrderBook, last models.Trade, vol float64) {
	if ob == nil {
		fmt.Printf("  %-5s (no book yet)\n", outcome)
		return
	}
	bid, _ := ob.BestBid()
	ask, _ := ob.BestAsk()
	mid := ob.MidPrice()
	fmt.Printf("  %-5s bid %.3f×%-7.0f ask %.3f×%-7.0f  spread %.3f  mid %.3f(=%.1f%%)  imb %+.2f  liq $%.0f  last %.3f  vol $%.0f\n",
		outcome, bid.Price, bid.Size, ask.Price, ask.Size,
		ob.Spread(), mid, mid*100, ob.Imbalance(5), ob.Liquidity(5), last.Price, vol)
}

func compact(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
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
