package api

import (
	"strconv"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// rawLevel is a price level as returned by the CLOB (string-encoded numbers).
type rawLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

func (l rawLevel) toModel() models.PriceLevel {
	return models.PriceLevel{
		Price: parseFloat(l.Price),
		Size:  parseFloat(l.Size),
	}
}

// rawOrderBook is the CLOB /book response.
type rawOrderBook struct {
	Market    string     `json:"market"`
	AssetID   string     `json:"asset_id"`
	Hash      string     `json:"hash"`
	Timestamp string     `json:"timestamp"`
	Bids      []rawLevel `json:"bids"`
	Asks      []rawLevel `json:"asks"`
}

func (r rawOrderBook) toModel() *models.OrderBook {
	ob := &models.OrderBook{
		TokenID:   r.AssetID,
		Hash:      r.Hash,
		Timestamp: parseMillis(r.Timestamp),
		Bids:      make([]models.PriceLevel, 0, len(r.Bids)),
		Asks:      make([]models.PriceLevel, 0, len(r.Asks)),
	}
	for _, b := range r.Bids {
		ob.Bids = append(ob.Bids, b.toModel())
	}
	for _, a := range r.Asks {
		ob.Asks = append(ob.Asks, a.toModel())
	}
	ob.Normalize()
	if ob.Timestamp.IsZero() {
		ob.Timestamp = time.Now()
	}
	return ob
}

// OrderRequest is the payload for placing an order on the CLOB.
//
// Polymarket orders are EIP-712 signed structures. This request carries the
// pre-signed order plus its owner/type metadata. When the bot runs without a
// wallet signer (dry-run), Signature is empty and the request is not sent.
type OrderRequest struct {
	Order     SignedOrder `json:"order"`
	Owner     string      `json:"owner"`
	OrderType string      `json:"orderType"`
}

// SignedOrder mirrors the CLOB signed-order schema.
type SignedOrder struct {
	Salt          string `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	Taker         string `json:"taker"`
	TokenID       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Expiration    string `json:"expiration"`
	Nonce         string `json:"nonce"`
	FeeRateBps    string `json:"feeRateBps"`
	Side          string `json:"side"`
	SignatureType int    `json:"signatureType"`
	Signature     string `json:"signature"`
}

// OrderResponse is the CLOB response to a placed order.
type OrderResponse struct {
	Success      bool     `json:"success"`
	ErrorMsg     string   `json:"errorMsg"`
	OrderID      string   `json:"orderID"`
	OrderHashes  []string `json:"orderHashes"`
	Status       string   `json:"status"`
	TakingAmount string   `json:"takingAmount"`
	MakingAmount string   `json:"makingAmount"`
}

// CancelRequest cancels a single order.
type CancelRequest struct {
	OrderID string `json:"orderID"`
}

// BalanceResponse is the CLOB balance-allowance response.
type BalanceResponse struct {
	Balance   string `json:"balance"`
	Allowance string `json:"allowance"`
}

// rawTrade is a CLOB trade record.
type rawTrade struct {
	ID        string `json:"id"`
	AssetID   string `json:"asset_id"`
	Side      string `json:"side"`
	Price     string `json:"price"`
	Size      string `json:"size"`
	MatchTime string `json:"match_time"`
}

func (t rawTrade) toModel() models.Trade {
	side := models.Buy
	if t.Side == "SELL" {
		side = models.Sell
	}
	return models.Trade{
		TokenID:   t.AssetID,
		Price:     parseFloat(t.Price),
		Size:      parseFloat(t.Size),
		Side:      side,
		Timestamp: parseMillis(t.MatchTime),
	}
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseMillis(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Some endpoints return RFC3339.
		if t, perr := time.Parse(time.RFC3339, s); perr == nil {
			return t
		}
		return time.Time{}
	}
	// Heuristic: treat large values as ms, smaller as seconds.
	if ms > 1e12 {
		return time.UnixMilli(ms)
	}
	return time.Unix(ms, 0)
}
