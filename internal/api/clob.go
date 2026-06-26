package api

import (
	"context"
	"fmt"
	"net/url"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// GetOrderBook fetches the current order book for a token.
func (c *Client) GetOrderBook(ctx context.Context, tokenID string) (*models.OrderBook, error) {
	var raw rawOrderBook
	path := "/book?token_id=" + url.QueryEscape(tokenID)
	if err := c.request(ctx, "GET", c.clobBase, path, nil, false, &raw); err != nil {
		return nil, fmt.Errorf("get order book %s: %w", tokenID, err)
	}
	if raw.AssetID == "" {
		raw.AssetID = tokenID
	}
	return raw.toModel(), nil
}

// GetMidpoint fetches the midpoint price for a token.
func (c *Client) GetMidpoint(ctx context.Context, tokenID string) (float64, error) {
	var resp struct {
		Mid string `json:"mid"`
	}
	path := "/midpoint?token_id=" + url.QueryEscape(tokenID)
	if err := c.request(ctx, "GET", c.clobBase, path, nil, false, &resp); err != nil {
		return 0, err
	}
	return parseFloat(resp.Mid), nil
}

// GetTrades fetches recent trades for a token (authenticated).
func (c *Client) GetTrades(ctx context.Context, tokenID string) ([]models.Trade, error) {
	var raw []rawTrade
	path := "/data/trades?market=" + url.QueryEscape(tokenID)
	if err := c.request(ctx, "GET", c.clobBase, path, nil, true, &raw); err != nil {
		return nil, fmt.Errorf("get trades: %w", err)
	}
	trades := make([]models.Trade, 0, len(raw))
	for _, t := range raw {
		trades = append(trades, t.toModel())
	}
	return trades, nil
}

// PlaceOrder submits a signed order to the CLOB.
func (c *Client) PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResponse, error) {
	var resp OrderResponse
	if err := c.request(ctx, "POST", c.clobBase, "/order", req, true, &resp); err != nil {
		return nil, fmt.Errorf("place order: %w", err)
	}
	if !resp.Success && resp.ErrorMsg != "" {
		return &resp, fmt.Errorf("order rejected: %s", resp.ErrorMsg)
	}
	return &resp, nil
}

// CancelOrder cancels a single resting order by id.
func (c *Client) CancelOrder(ctx context.Context, orderID string) error {
	var resp struct {
		Canceled []string          `json:"canceled"`
		NotFound []string          `json:"not_canceled"`
		Errors   map[string]string `json:"errors"`
	}
	body := CancelRequest{OrderID: orderID}
	if err := c.request(ctx, "DELETE", c.clobBase, "/order", body, true, &resp); err != nil {
		return fmt.Errorf("cancel order %s: %w", orderID, err)
	}
	return nil
}

// CancelAll cancels every open order for the account.
func (c *Client) CancelAll(ctx context.Context) error {
	if err := c.request(ctx, "DELETE", c.clobBase, "/cancel-all", nil, true, nil); err != nil {
		return fmt.Errorf("cancel all: %w", err)
	}
	return nil
}

// GetBalance fetches USDC collateral balance and allowance.
func (c *Client) GetBalance(ctx context.Context) (*BalanceResponse, error) {
	var resp BalanceResponse
	path := "/balance-allowance?asset_type=COLLATERAL"
	if err := c.request(ctx, "GET", c.clobBase, path, nil, true, &resp); err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}
	return &resp, nil
}

// GetOrderStatus fetches the status of a single order by id.
func (c *Client) GetOrderStatus(ctx context.Context, orderID string) (*OrderResponse, error) {
	var resp OrderResponse
	path := "/data/order/" + url.PathEscape(orderID)
	if err := c.request(ctx, "GET", c.clobBase, path, nil, true, &resp); err != nil {
		return nil, fmt.Errorf("get order status %s: %w", orderID, err)
	}
	return &resp, nil
}
