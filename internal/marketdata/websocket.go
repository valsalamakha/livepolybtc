package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// wsSubscription is the subscribe payload for the public market channel.
type wsSubscription struct {
	Type      string   `json:"type"`
	AssetsIDs []string `json:"assets_ids"`
}

// wsMessage captures the union of fields across the market-channel event types
// the bot cares about (book snapshots, price deltas, last trade).
type wsMessage struct {
	EventType string `json:"event_type"`
	AssetID   string `json:"asset_id"`
	Market    string `json:"market"`
	Hash      string `json:"hash"`
	Timestamp string `json:"timestamp"`
	Bids      []struct {
		Price string `json:"price"`
		Size  string `json:"size"`
	} `json:"bids"`
	Asks []struct {
		Price string `json:"price"`
		Size  string `json:"size"`
	} `json:"asks"`
	Changes []struct {
		Price string `json:"price"`
		Size  string `json:"size"`
		Side  string `json:"side"`
	} `json:"changes"`
	Price string `json:"price"`
	Size  string `json:"size"`
	Side  string `json:"side"`
}

// runWebsocket connects to the market channel, subscribes to the active
// market's tokens, and applies updates to the collector until the context is
// cancelled. It returns on error so the caller can manage reconnection.
func (c *Collector) runWebsocket(ctx context.Context, tokenIDs []string) error {
	url := c.wsBaseURL + "/market"
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            websocket.DefaultDialer.Proxy,
	}
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("ws dial %s: %w", url, err)
	}
	defer conn.Close()

	sub := wsSubscription{Type: "market", AssetsIDs: tokenIDs}
	if err := conn.WriteJSON(sub); err != nil {
		return fmt.Errorf("ws subscribe: %w", err)
	}

	// Reader goroutine feeds messages to the select loop so we can honour ctx.
	type readResult struct {
		data []byte
		err  error
	}
	reads := make(chan readResult, 4)
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			select {
			case reads <- readResult{data: data, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// Periodic ping to keep the connection alive.
	ping := time.NewTicker(10 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ping.C:
			_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
		case r := <-reads:
			if r.err != nil {
				return fmt.Errorf("ws read: %w", r.err)
			}
			c.markConnected()
			c.handleMessage(r.data)
		}
	}
}

// handleMessage parses one raw frame, which may be a single event object or an
// array of events.
func (c *Collector) handleMessage(data []byte) {
	if c.RawHandler != nil {
		c.RawHandler(data)
	}
	trimmed := trimSpace(data)
	if len(trimmed) == 0 {
		return
	}
	if trimmed[0] == '[' {
		var msgs []wsMessage
		if err := json.Unmarshal(trimmed, &msgs); err != nil {
			return
		}
		for i := range msgs {
			c.applyMessage(&msgs[i])
		}
		return
	}
	var msg wsMessage
	if err := json.Unmarshal(trimmed, &msg); err != nil {
		return
	}
	c.applyMessage(&msg)
}

func (c *Collector) applyMessage(msg *wsMessage) {
	switch msg.EventType {
	case "book":
		c.applyBookSnapshot(msg)
	case "price_change":
		c.applyPriceChange(msg)
	case "last_trade_price", "last_trade":
		c.applyLastTrade(msg)
	}
}

func (c *Collector) applyBookSnapshot(msg *wsMessage) {
	ob := &models.OrderBook{
		TokenID:   msg.AssetID,
		Hash:      msg.Hash,
		Timestamp: parseWSTime(msg.Timestamp),
	}
	for _, b := range msg.Bids {
		ob.Bids = append(ob.Bids, models.PriceLevel{Price: atof(b.Price), Size: atof(b.Size)})
	}
	for _, a := range msg.Asks {
		ob.Asks = append(ob.Asks, models.PriceLevel{Price: atof(a.Price), Size: atof(a.Size)})
	}
	ob.Normalize()
	if ob.Timestamp.IsZero() {
		ob.Timestamp = time.Now()
	}
	c.setBook(ob)
}

func (c *Collector) applyPriceChange(msg *wsMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ob := c.books[msg.AssetID]
	if ob == nil {
		ob = &models.OrderBook{TokenID: msg.AssetID}
		c.books[msg.AssetID] = ob
	}
	for _, ch := range msg.Changes {
		price := atof(ch.Price)
		size := atof(ch.Size)
		if ch.Side == "BUY" {
			ob.Bids = upsertLevel(ob.Bids, price, size)
		} else {
			ob.Asks = upsertLevel(ob.Asks, price, size)
		}
	}
	ob.Timestamp = time.Now()
	ob.Normalize()
}

func (c *Collector) applyLastTrade(msg *wsMessage) {
	price := atof(msg.Price)
	size := atof(msg.Size)
	if price <= 0 {
		return
	}
	side := models.Buy
	if msg.Side == "SELL" {
		side = models.Sell
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastTrades[msg.AssetID] = models.Trade{
		TokenID:   msg.AssetID,
		Price:     price,
		Size:      size,
		Side:      side,
		Timestamp: time.Now(),
	}
	c.volume[msg.AssetID] += price * size
}

// upsertLevel sets or removes a price level (size 0 removes it).
func upsertLevel(levels []models.PriceLevel, price, size float64) []models.PriceLevel {
	for i := range levels {
		if levels[i].Price == price {
			if size == 0 {
				return append(levels[:i], levels[i+1:]...)
			}
			levels[i].Size = size
			return levels
		}
	}
	if size == 0 {
		return levels
	}
	return append(levels, models.PriceLevel{Price: price, Size: size})
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseWSTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		if ms > 1e12 {
			return time.UnixMilli(ms)
		}
		return time.Unix(ms, 0)
	}
	return time.Time{}
}

func trimSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	j := len(b)
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == '\t') {
		j--
	}
	return b[i:j]
}
