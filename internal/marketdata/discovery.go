// Package marketdata handles automatic market discovery and continuous order
// book collection (via websocket with a REST polling backstop), producing the
// snapshots the strategy consumes.
package marketdata

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/config"
	"github.com/valsalamakha/livepolybtc/internal/models"
)

// Discoverer locates the currently active Bitcoin 5-minute up/down market
// without any hard-coded token ids.
type Discoverer struct {
	client *api.Client
	cfg    config.MarketConfig
}

// NewDiscoverer constructs a Discoverer.
func NewDiscoverer(client *api.Client, cfg config.MarketConfig) *Discoverer {
	return &Discoverer{client: client, cfg: cfg}
}

// Discover returns the active market whose question matches the configured
// keywords (and optional series), resolving soonest in the future. It is the
// market the bot should currently be trading.
func (d *Discoverer) Discover(ctx context.Context) (*models.Market, error) {
	active, closed := true, false
	params := api.ListMarketsParams{
		Active:     &active,
		Closed:     &closed,
		Limit:      500,
		Order:      "endDate",
		Ascending:  ptr(true),
		SeriesSlug: d.cfg.SeriesSlug,
	}
	markets, err := d.client.ListMarkets(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}

	now := time.Now()
	candidates := make([]*models.Market, 0, len(markets))
	for _, m := range markets {
		if !d.matches(m) {
			continue
		}
		if len(m.Tokens) != 2 {
			continue
		}
		// Keep markets that have not yet resolved (allow a small grace so we
		// can still discover one that just ticked over).
		if !m.EndTime.IsZero() && m.EndTime.Before(now.Add(-5*time.Second)) {
			continue
		}
		candidates = append(candidates, m)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no active market matched keywords %v", d.cfg.Keywords)
	}

	// Prefer the nearest future resolution (the live window).
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].EndTime.Before(candidates[j].EndTime)
	})
	return candidates[0], nil
}

// matches reports whether a market's question contains all configured keywords.
func (d *Discoverer) matches(m *models.Market) bool {
	q := strings.ToLower(m.Question + " " + m.Slug)
	for _, kw := range d.cfg.Keywords {
		if !strings.Contains(q, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}

func ptr[T any](v T) *T { return &v }
