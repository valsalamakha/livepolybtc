package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/logging"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

func testCollector(wsURL string) *Collector {
	log := logging.New(logging.LevelError, false)
	poly := config.PolymarketConfig{WSBaseURL: wsURL}
	health := config.HealthConfig{ReconnectBackoff: 10 * time.Millisecond, MaxReconnectBackoff: 50 * time.Millisecond}
	return NewCollector(nil, log, poly, health, true)
}

// TestWebsocketAppliesBookSnapshot is an integration test against a mock
// websocket server that speaks the Polymarket market-channel protocol.
func TestWebsocketAppliesBookSnapshot(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/market") {
			t.Errorf("unexpected ws path %s", r.URL.Path)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Read the subscription frame.
		var sub wsSubscription
		if err := conn.ReadJSON(&sub); err != nil {
			return
		}
		if len(sub.AssetsIDs) == 0 {
			t.Error("subscription had no asset ids")
		}
		// Push a book snapshot for the first subscribed token.
		_ = conn.WriteJSON(map[string]any{
			"event_type": "book",
			"asset_id":   sub.AssetsIDs[0],
			"bids":       []map[string]string{{"price": "0.45", "size": "200"}},
			"asks":       []map[string]string{{"price": "0.55", "size": "100"}},
		})
		// Keep the connection open until the client disconnects.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c := testCollector(wsURL)
	c.SetMarket(&models.Market{
		ConditionID: "0xC",
		Tokens:      []models.Token{{TokenID: "TOK1", Outcome: "Up"}, {TokenID: "TOK2", Outcome: "Down"}},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go c.runWebsocket(ctx, []string{"TOK1", "TOK2"})

	// Wait for the book to arrive.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := c.Snapshot(models.BTCStats{})
		if ob := snap.Book("TOK1"); ob != nil {
			if bb, ok := ob.BestBid(); ok && bb.Price == 0.45 {
				return // success
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("book snapshot was not applied within timeout")
}

func TestApplyPriceChangeDelta(t *testing.T) {
	c := testCollector("ws://unused")
	c.SetMarket(&models.Market{Tokens: []models.Token{{TokenID: "T"}}})
	c.applyBookSnapshot(&wsMessage{
		EventType: "book", AssetID: "T",
		Bids: []struct {
			Price string `json:"price"`
			Size  string `json:"size"`
		}{{"0.40", "100"}},
		Asks: []struct {
			Price string `json:"price"`
			Size  string `json:"size"`
		}{{"0.60", "100"}},
	})
	// Update the bid level and remove the ask level.
	c.applyPriceChange(&wsMessage{
		EventType: "price_change", AssetID: "T",
		Changes: []struct {
			Price string `json:"price"`
			Size  string `json:"size"`
			Side  string `json:"side"`
		}{
			{"0.40", "250", "BUY"},
			{"0.60", "0", "SELL"},
		},
	})
	snap := c.Snapshot(models.BTCStats{})
	ob := snap.Book("T")
	if bb, ok := ob.BestBid(); !ok || bb.Size != 250 {
		t.Errorf("bid size = %+v ok=%v, want 250", bb, ok)
	}
	if _, ok := ob.BestAsk(); ok {
		t.Error("ask should have been removed by size-0 delta")
	}
}

func TestApplyLastTradeUpdatesVolume(t *testing.T) {
	c := testCollector("ws://unused")
	c.SetMarket(&models.Market{Tokens: []models.Token{{TokenID: "T"}}})
	c.applyLastTrade(&wsMessage{EventType: "last_trade_price", AssetID: "T", Price: "0.5", Size: "10", Side: "BUY"})
	snap := c.Snapshot(models.BTCStats{})
	if tr, ok := snap.LastTrades["T"]; !ok || tr.Price != 0.5 {
		t.Errorf("last trade = %+v ok=%v", tr, ok)
	}
	if snap.Volume["T"] != 5.0 {
		t.Errorf("volume = %v, want 5.0", snap.Volume["T"])
	}
}

func TestHandleMessageArray(t *testing.T) {
	c := testCollector("ws://unused")
	c.SetMarket(&models.Market{Tokens: []models.Token{{TokenID: "T"}}})
	c.handleMessage([]byte(`[{"event_type":"book","asset_id":"T","bids":[{"price":"0.3","size":"50"}],"asks":[{"price":"0.7","size":"50"}]}]`))
	snap := c.Snapshot(models.BTCStats{})
	if ob := snap.Book("T"); ob == nil {
		t.Fatal("book not applied from array message")
	}
}
