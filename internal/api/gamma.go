package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/models"
)

// gammaMarket mirrors the relevant fields of a Gamma /markets entry. Several
// numeric/array fields are JSON-encoded strings, a Gamma quirk handled below.
type gammaMarket struct {
	ID              string `json:"id"`
	Question        string `json:"question"`
	QuestionID      string `json:"questionID"`
	ConditionID     string `json:"conditionId"`
	Slug            string `json:"slug"`
	EndDate         string `json:"endDate"`
	StartDate       string `json:"startDate"`
	Active          bool   `json:"active"`
	Closed          bool   `json:"closed"`
	AcceptingOrders bool   `json:"acceptingOrders"`
	CLOBTokenIDs    string `json:"clobTokenIds"`
	Outcomes        string `json:"outcomes"`
	OutcomePrices   string `json:"outcomePrices"`
	MinTickSize     string `json:"orderPriceMinTickSize"`
	MinOrderSize    string `json:"orderMinSize"`
}

func (g gammaMarket) toModel() (*models.Market, error) {
	var tokenIDs, outcomes, prices []string
	if g.CLOBTokenIDs != "" {
		if err := json.Unmarshal([]byte(g.CLOBTokenIDs), &tokenIDs); err != nil {
			return nil, fmt.Errorf("parse clobTokenIds: %w", err)
		}
	}
	if g.Outcomes != "" {
		_ = json.Unmarshal([]byte(g.Outcomes), &outcomes)
	}
	if g.OutcomePrices != "" {
		_ = json.Unmarshal([]byte(g.OutcomePrices), &prices)
	}

	tokens := make([]models.Token, 0, len(tokenIDs))
	for i, id := range tokenIDs {
		t := models.Token{TokenID: id}
		if i < len(outcomes) {
			t.Outcome = outcomes[i]
		}
		if i < len(prices) {
			t.Price = parseFloat(prices[i])
		}
		tokens = append(tokens, t)
	}

	m := &models.Market{
		ConditionID:     g.ConditionID,
		QuestionID:      g.QuestionID,
		Slug:            g.Slug,
		Question:        g.Question,
		Tokens:          tokens,
		Active:          g.Active,
		Closed:          g.Closed,
		AcceptingOrders: g.AcceptingOrders,
		MinTickSize:     parseFloat(g.MinTickSize),
		MinOrderSize:    parseFloat(g.MinOrderSize),
		StartTime:       parseRFC(g.StartDate),
		EndTime:         parseRFC(g.EndDate),
	}
	return m, nil
}

// ListMarketsParams narrows a Gamma market query.
type ListMarketsParams struct {
	Active     *bool
	Closed     *bool
	Limit      int
	Order      string
	Ascending  *bool
	SeriesSlug string
	TagSlug    string
}

// ListMarkets queries the Gamma markets endpoint.
func (c *Client) ListMarkets(ctx context.Context, p ListMarketsParams) ([]*models.Market, error) {
	q := url.Values{}
	if p.Active != nil {
		q.Set("active", boolStr(*p.Active))
	}
	if p.Closed != nil {
		q.Set("closed", boolStr(*p.Closed))
	}
	if p.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", p.Limit))
	}
	if p.Order != "" {
		q.Set("order", p.Order)
	}
	if p.Ascending != nil {
		q.Set("ascending", boolStr(*p.Ascending))
	}
	if p.SeriesSlug != "" {
		q.Set("series_slug", p.SeriesSlug)
	}
	if p.TagSlug != "" {
		q.Set("tag_slug", p.TagSlug)
	}

	path := "/markets"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	var raw []gammaMarket
	if err := c.request(ctx, "GET", c.gammaBase, path, nil, false, &raw); err != nil {
		return nil, fmt.Errorf("list markets: %w", err)
	}

	out := make([]*models.Market, 0, len(raw))
	for _, g := range raw {
		m, err := g.toModel()
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func parseRFC(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
