package btcfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Source fetches the current BTC spot price from a specific exchange.
type Source interface {
	// Name returns the exchange identifier.
	Name() string
	// Fetch returns the latest spot price.
	Fetch(ctx context.Context) (float64, error)
}

// NewSource constructs a price source for the given exchange and symbol.
func NewSource(exchange, symbol string, hc *http.Client) (Source, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	switch exchange {
	case "coinbase":
		if symbol == "" {
			symbol = "BTC-USD"
		}
		return &coinbaseSource{symbol: symbol, hc: hc}, nil
	case "binance":
		if symbol == "" {
			symbol = "BTCUSDT"
		}
		return &binanceSource{symbol: symbol, hc: hc}, nil
	case "kraken":
		if symbol == "" {
			symbol = "XBTUSD"
		}
		return &krakenSource{pair: symbol, hc: hc}, nil
	default:
		return nil, fmt.Errorf("unsupported exchange %q", exchange)
	}
}

func getJSON(ctx context.Context, hc *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, out)
}

// --- Coinbase ---

type coinbaseSource struct {
	symbol string
	hc     *http.Client
}

func (s *coinbaseSource) Name() string { return "coinbase" }

func (s *coinbaseSource) Fetch(ctx context.Context) (float64, error) {
	var resp struct {
		Price string `json:"price"`
	}
	url := "https://api.exchange.coinbase.com/products/" + s.symbol + "/ticker"
	if err := getJSON(ctx, s.hc, url, &resp); err != nil {
		return 0, fmt.Errorf("coinbase fetch: %w", err)
	}
	p, err := strconv.ParseFloat(resp.Price, 64)
	if err != nil {
		return 0, fmt.Errorf("coinbase parse price %q: %w", resp.Price, err)
	}
	return p, nil
}

// --- Binance ---

type binanceSource struct {
	symbol string
	hc     *http.Client
}

func (s *binanceSource) Name() string { return "binance" }

func (s *binanceSource) Fetch(ctx context.Context) (float64, error) {
	var resp struct {
		Price string `json:"price"`
	}
	url := "https://api.binance.com/api/v3/ticker/price?symbol=" + s.symbol
	if err := getJSON(ctx, s.hc, url, &resp); err != nil {
		return 0, fmt.Errorf("binance fetch: %w", err)
	}
	p, err := strconv.ParseFloat(resp.Price, 64)
	if err != nil {
		return 0, fmt.Errorf("binance parse price %q: %w", resp.Price, err)
	}
	return p, nil
}

// --- Kraken ---

type krakenSource struct {
	pair string
	hc   *http.Client
}

func (s *krakenSource) Name() string { return "kraken" }

func (s *krakenSource) Fetch(ctx context.Context) (float64, error) {
	var resp struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			C []string `json:"c"` // [last trade price, lot volume]
		} `json:"result"`
	}
	url := "https://api.kraken.com/0/public/Ticker?pair=" + s.pair
	if err := getJSON(ctx, s.hc, url, &resp); err != nil {
		return 0, fmt.Errorf("kraken fetch: %w", err)
	}
	if len(resp.Error) > 0 {
		return 0, fmt.Errorf("kraken error: %v", resp.Error)
	}
	for _, v := range resp.Result {
		if len(v.C) > 0 {
			p, err := strconv.ParseFloat(v.C[0], 64)
			if err != nil {
				return 0, fmt.Errorf("kraken parse price %q: %w", v.C[0], err)
			}
			return p, nil
		}
	}
	return 0, fmt.Errorf("kraken: no result")
}
