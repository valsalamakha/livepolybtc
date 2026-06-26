// Package backtest provides a historical replay framework for the strategy:
// it feeds recorded market/BTC data through the live strategy engine and a
// simulated executor, then reports performance metrics.
package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Tick is a single point-in-time observation used for replay. Each market is
// represented by two single-or-multi-level books (up/down) plus the BTC spot.
type Tick struct {
	Time        time.Time
	ConditionID string
	EndTime     time.Time
	UpToken     string
	DownToken   string
	UpBook      *models.OrderBook
	DownBook    *models.OrderBook
	BTCPrice    float64
}

// Snapshot builds a strategy snapshot from the tick using the supplied BTC
// stats (computed externally so volatility/move windows are consistent).
func (t Tick) Snapshot(btc models.BTCStats) models.Snapshot {
	m := &models.Market{
		ConditionID: t.ConditionID,
		EndTime:     t.EndTime,
		Tokens: []models.Token{
			{TokenID: t.UpToken, Outcome: "Up"},
			{TokenID: t.DownToken, Outcome: "Down"},
		},
	}
	books := map[string]*models.OrderBook{
		t.UpToken:   t.UpBook,
		t.DownToken: t.DownBook,
	}
	return models.Snapshot{
		Market:           m,
		Books:            books,
		BTC:              btc,
		TimeToResolution: t.EndTime.Sub(t.Time),
		Timestamp:        t.Time,
	}
}

// LoadCSV reads ticks from a CSV file with the header:
//
//	timestamp,condition_id,end_time,up_token,down_token,
//	up_bid,up_bid_size,up_ask,up_ask_size,
//	down_bid,down_bid_size,down_ask,down_ask_size,btc_price
//
// Timestamps may be RFC3339 or unix seconds.
func LoadCSV(path string) ([]Tick, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseCSV(f)
}

// ParseCSV parses ticks from a reader.
func ParseCSV(r io.Reader) ([]Tick, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("csv has no data rows")
	}

	const want = 14
	ticks := make([]Tick, 0, len(rows)-1)
	for i, row := range rows[1:] {
		if len(row) < want {
			return nil, fmt.Errorf("row %d: expected %d columns, got %d", i+2, want, len(row))
		}
		t := Tick{
			Time:        parseTime(row[0]),
			ConditionID: row[1],
			EndTime:     parseTime(row[2]),
			UpToken:     row[3],
			DownToken:   row[4],
			BTCPrice:    atof(row[13]),
		}
		t.UpBook = bookFrom(t.UpToken, t.Time, atof(row[5]), atof(row[6]), atof(row[7]), atof(row[8]))
		t.DownBook = bookFrom(t.DownToken, t.Time, atof(row[9]), atof(row[10]), atof(row[11]), atof(row[12]))
		ticks = append(ticks, t)
	}

	sort.SliceStable(ticks, func(i, j int) bool { return ticks[i].Time.Before(ticks[j].Time) })
	return ticks, nil
}

var csvHeader = []string{
	"timestamp", "condition_id", "end_time", "up_token", "down_token",
	"up_bid", "up_bid_size", "up_ask", "up_ask_size",
	"down_bid", "down_bid_size", "down_ask", "down_ask_size", "btc_price",
}

// WriteCSV writes ticks to path in the canonical replay schema (the same schema
// LoadCSV reads). It is handy for capturing live data or exporting synthetic
// datasets for sharing.
func WriteCSV(path string, ticks []Tick) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	ff := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	for _, tk := range ticks {
		ub, ubs, ua, uas := bestLevels(tk.UpBook)
		db, dbs, da, das := bestLevels(tk.DownBook)
		row := []string{
			tk.Time.UTC().Format(time.RFC3339), tk.ConditionID, tk.EndTime.UTC().Format(time.RFC3339),
			tk.UpToken, tk.DownToken,
			ff(ub), ff(ubs), ff(ua), ff(uas),
			ff(db), ff(dbs), ff(da), ff(das), ff(tk.BTCPrice),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func bestLevels(ob *models.OrderBook) (bid, bidSz, ask, askSz float64) {
	if ob == nil {
		return 0, 0, 0, 0
	}
	if b, ok := ob.BestBid(); ok {
		bid, bidSz = b.Price, b.Size
	}
	if a, ok := ob.BestAsk(); ok {
		ask, askSz = a.Price, a.Size
	}
	return bid, bidSz, ask, askSz
}

func bookFrom(token string, ts time.Time, bid, bidSz, ask, askSz float64) *models.OrderBook {
	ob := &models.OrderBook{TokenID: token, Timestamp: ts}
	if bid > 0 && bidSz > 0 {
		ob.Bids = append(ob.Bids, models.PriceLevel{Price: bid, Size: bidSz})
	}
	if ask > 0 && askSz > 0 {
		ob.Asks = append(ob.Asks, models.PriceLevel{Price: ask, Size: askSz})
	}
	ob.Normalize()
	return ob
}

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if secs, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(secs)
		nsec := int64((secs - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC()
	}
	return time.Time{}
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
