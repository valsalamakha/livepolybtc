package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/btcfeed"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
)

// constSource is a btcfeed.Source returning a fixed price (no network).
type constSource struct{ price float64 }

func (c constSource) Name() string                           { return "const" }
func (c constSource) Fetch(context.Context) (float64, error) { return c.price, nil }

// mockPolymarket serves the minimal gamma + clob endpoints the bot touches.
func mockPolymarket(t *testing.T) *httptest.Server {
	end := time.Now().Add(1500 * time.Millisecond).UTC().Format(time.RFC3339)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/markets"):
			w.Write([]byte(`[{
				"id":"1","question":"Bitcoin Up or Down?","conditionId":"0xCOND","slug":"btc-up-or-down",
				"endDate":"` + end + `","active":true,"closed":false,"acceptingOrders":true,
				"clobTokenIds":"[\"UP\",\"DOWN\"]","outcomes":"[\"Up\",\"Down\"]","outcomePrices":"[\"0.95\",\"0.05\"]"
			}]`))
		case strings.HasSuffix(r.URL.Path, "/book"):
			tok := r.URL.Query().Get("token_id")
			if tok == "UP" {
				w.Write([]byte(`{"asset_id":"UP","bids":[{"price":"0.94","size":"5000"}],"asks":[{"price":"0.96","size":"80"}]}`))
			} else {
				w.Write([]byte(`{"asset_id":"DOWN","bids":[{"price":"0.04","size":"100"}],"asks":[{"price":"0.06","size":"2000"}]}`))
			}
		case strings.Contains(r.URL.Path, "/midpoint"):
			w.Write([]byte(`{"mid":"0.99"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestBotDiscoversAndShutsDownGracefully(t *testing.T) {
	srv := mockPolymarket(t)
	defer srv.Close()

	cfg := config.Default()
	cfg.DryRun = true
	cfg.Polymarket.RestBaseURL = srv.URL
	cfg.Polymarket.GammaBaseURL = srv.URL
	cfg.BTCFeed.UseWebsocket = false // polling only; no websocket server in this test
	cfg.BTCFeed.PollInterval = 100 * time.Millisecond
	cfg.Market.DiscoveryInterval = 200 * time.Millisecond
	cfg.Health.HeartbeatInterval = 200 * time.Millisecond
	cfg.ApplyDerived()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config invalid: %v", err)
	}

	log := logging.New(logging.LevelError, false)
	bot, err := New(cfg, log, nil)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	// Inject a non-networked BTC source.
	bot.feed = btcfeed.NewWithSource(constSource{price: 60000}, cfg.BTCFeed)

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	// Give discovery + polling time to populate the collector.
	deadline := time.Now().Add(600 * time.Millisecond)
	var discovered bool
	for time.Now().Before(deadline) {
		if bot.coll.Market() != nil {
			snap := bot.coll.Snapshot(bot.feed.Stats())
			if snap.Book("UP") != nil {
				discovered = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !discovered {
		t.Error("bot did not discover the market / collect books via polling")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("bot.Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bot.Run did not shut down within timeout")
	}
}
