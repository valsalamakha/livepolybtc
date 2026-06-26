package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/valsalamakha/livepolybtc/internal/auth"
)

func newTestSigner() *auth.Signer {
	sec := base64.URLEncoding.EncodeToString([]byte("secret"))
	return auth.NewSigner(auth.Credentials{APIKey: "k", Secret: sec, Passphrase: "p", Address: "0xabc"})
}

func TestGetOrderBookParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{
			"market":"0xM","asset_id":"TOK","hash":"h","timestamp":"1700000000000",
			"bids":[{"price":"0.45","size":"200"},{"price":"0.40","size":"100"}],
			"asks":[{"price":"0.55","size":"150"},{"price":"0.50","size":"300"}]
		}`))
	}))
	defer srv.Close()

	c := New(Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL})
	ob, err := c.GetOrderBook(context.Background(), "TOK")
	if err != nil {
		t.Fatalf("get order book: %v", err)
	}
	if ob.TokenID != "TOK" {
		t.Errorf("token id = %s, want TOK", ob.TokenID)
	}
	if bb, ok := ob.BestBid(); !ok || bb.Price != 0.45 {
		t.Errorf("best bid = %+v ok=%v, want 0.45", bb, ok)
	}
	if ba, ok := ob.BestAsk(); !ok || ba.Price != 0.50 {
		t.Errorf("best ask = %+v ok=%v, want 0.50", ba, ok)
	}
}

func TestListMarketsParsesGammaQuirks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{
			"id":"1","question":"Bitcoin Up or Down?","conditionId":"0xCOND","slug":"btc-up-or-down",
			"endDate":"2026-06-26T12:05:00Z","active":true,"closed":false,"acceptingOrders":true,
			"clobTokenIds":"[\"111\",\"222\"]","outcomes":"[\"Up\",\"Down\"]","outcomePrices":"[\"0.97\",\"0.03\"]"
		}]`))
	}))
	defer srv.Close()

	c := New(Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL})
	markets, err := c.ListMarkets(context.Background(), ListMarketsParams{})
	if err != nil {
		t.Fatalf("list markets: %v", err)
	}
	if len(markets) != 1 {
		t.Fatalf("got %d markets, want 1", len(markets))
	}
	m := markets[0]
	if len(m.Tokens) != 2 {
		t.Fatalf("got %d tokens, want 2", len(m.Tokens))
	}
	if m.Tokens[0].TokenID != "111" || m.Tokens[0].Outcome != "Up" {
		t.Errorf("token[0] = %+v, want {111, Up}", m.Tokens[0])
	}
	if m.Tokens[1].Price != 0.03 {
		t.Errorf("token[1] price = %v, want 0.03", m.Tokens[1].Price)
	}
}

func TestPlaceOrderSendsAuthHeaders(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Write([]byte(`{"success":true,"orderID":"OID","status":"matched","takingAmount":"100000000"}`))
	}))
	defer srv.Close()

	c := New(Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL, Signer: newTestSigner()})
	resp, err := c.PlaceOrder(context.Background(), OrderRequest{Owner: "0xabc", OrderType: "FAK"})
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	if !resp.Success || resp.OrderID != "OID" {
		t.Errorf("unexpected response: %+v", resp)
	}
	for _, h := range []string{auth.HeaderSignature, auth.HeaderTimestamp, auth.HeaderAPIKey, auth.HeaderPassphrase} {
		if gotHeaders.Get(h) == "" {
			t.Errorf("missing auth header %s on signed request", h)
		}
	}
}

func TestRetryOnServerError(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"market":"0xM","asset_id":"TOK","bids":[],"asks":[]}`))
	}))
	defer srv.Close()

	c := New(Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL, MaxRetries: 3})
	if _, err := c.GetOrderBook(context.Background(), "TOK"); err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (2 failures + 1 success)", calls)
	}
}

func TestNonRetryableError(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL, MaxRetries: 3})
	if _, err := c.GetOrderBook(context.Background(), "TOK"); err == nil {
		t.Fatal("expected error on 400")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (400 is not retryable)", calls)
	}
}
