package marketdata

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Collector continuously maintains order books, last trades and volume for the
// active market's tokens. It prefers a websocket feed and uses a periodic REST
// poll as a backstop. It is safe for concurrent use.
type Collector struct {
	client    *api.Client
	log       *logging.Logger
	wsBaseURL string
	health    config.HealthConfig
	useWS     bool

	mu         sync.RWMutex
	market     *models.Market
	books      map[string]*models.OrderBook
	lastTrades map[string]models.Trade
	volume     map[string]float64
	connected  bool
	lastUpdate time.Time

	// RawHandler, when set, is invoked with every raw websocket frame before it
	// is parsed. It is used by diagnostics to dump the live stream. It must not
	// block.
	RawHandler func([]byte)
}

// NewCollector constructs a Collector.
func NewCollector(client *api.Client, log *logging.Logger, poly config.PolymarketConfig, health config.HealthConfig, useWS bool) *Collector {
	return &Collector{
		client:     client,
		log:        log,
		wsBaseURL:  strings.TrimRight(poly.WSBaseURL, "/"),
		health:     health,
		useWS:      useWS,
		books:      make(map[string]*models.OrderBook),
		lastTrades: make(map[string]models.Trade),
		volume:     make(map[string]float64),
	}
}

// SetMarket installs the active market and resets per-market state.
func (c *Collector) SetMarket(m *models.Market) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.market != nil && c.market.ConditionID == m.ConditionID {
		return
	}
	c.market = m
	c.books = make(map[string]*models.OrderBook)
	c.lastTrades = make(map[string]models.Trade)
	c.volume = make(map[string]float64)
}

// Market returns the active market.
func (c *Collector) Market() *models.Market {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.market
}

// Run drives collection for the active market: it primes the books with a REST
// snapshot, then runs the websocket loop (with reconnect) and a polling
// backstop concurrently until the context is cancelled.
func (c *Collector) Run(ctx context.Context) {
	market := c.Market()
	if market == nil {
		return
	}
	tokenIDs := market.TokenIDs()

	// Prime synchronously so a snapshot is available immediately.
	c.pollBooks(ctx, tokenIDs)

	var wg sync.WaitGroup
	if c.useWS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.websocketLoop(ctx, tokenIDs)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.pollLoop(ctx, tokenIDs)
	}()

	wg.Wait()
}

// websocketLoop maintains the websocket connection with exponential backoff
// reconnection.
func (c *Collector) websocketLoop(ctx context.Context, tokenIDs []string) {
	backoff := c.health.ReconnectBackoff
	if backoff <= 0 {
		backoff = time.Second
	}
	attempts := 0
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.runWebsocket(ctx, tokenIDs)
		if ctx.Err() != nil {
			return
		}
		c.markDisconnected()
		attempts++
		if c.health.MaxReconnectAttempts > 0 && attempts >= c.health.MaxReconnectAttempts {
			c.log.Errorf("marketdata websocket: giving up after %d attempts: %v", attempts, err)
			return
		}
		c.log.Warnf("marketdata websocket disconnected (%v); reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > c.health.MaxReconnectBackoff && c.health.MaxReconnectBackoff > 0 {
			backoff = c.health.MaxReconnectBackoff
		}
	}
}

// pollLoop periodically refreshes the books via REST. When websockets are
// enabled this is a low-frequency backstop; otherwise it is the primary feed.
func (c *Collector) pollLoop(ctx context.Context, tokenIDs []string) {
	interval := 1 * time.Second
	if c.useWS {
		interval = 3 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.pollBooks(ctx, tokenIDs)
		}
	}
}

func (c *Collector) pollBooks(ctx context.Context, tokenIDs []string) {
	for _, id := range tokenIDs {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ob, err := c.client.GetOrderBook(cctx, id)
		cancel()
		if err != nil {
			c.log.Debugf("poll book %s: %v", id, err)
			continue
		}
		c.setBook(ob)
	}
}

func (c *Collector) setBook(ob *models.OrderBook) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.books[ob.TokenID] = ob
	c.lastUpdate = time.Now()
}

func (c *Collector) markConnected() {
	c.mu.Lock()
	c.connected = true
	c.lastUpdate = time.Now()
	c.mu.Unlock()
}

func (c *Collector) markDisconnected() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// Snapshot assembles the current market data plus the supplied BTC stats into
// a strategy snapshot.
func (c *Collector) Snapshot(btc models.BTCStats) models.Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	books := make(map[string]*models.OrderBook, len(c.books))
	for id, ob := range c.books {
		books[id] = ob.Clone()
	}
	trades := make(map[string]models.Trade, len(c.lastTrades))
	for id, t := range c.lastTrades {
		trades[id] = t
	}
	vol := make(map[string]float64, len(c.volume))
	for id, v := range c.volume {
		vol[id] = v
	}
	var ttr time.Duration
	if c.market != nil {
		ttr = c.market.TimeToResolution(now)
	}
	return models.Snapshot{
		Market:           c.market,
		Books:            books,
		LastTrades:       trades,
		Volume:           vol,
		BTC:              btc,
		TimeToResolution: ttr,
		Timestamp:        now,
	}
}

// Health reports the collector's connection state and time since last update.
func (c *Collector) Health() (connected bool, sinceUpdate time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected, time.Since(c.lastUpdate)
}
