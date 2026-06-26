package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/valsalamakha/livepolybtc/internal/api"
	"github.com/valsalamakha/livepolybtc/internal/config"
)

func TestDiscoverPicksNearestMatchingMarket(t *testing.T) {
	// Two BTC up/down markets (one sooner) plus an unrelated market.
	soon := time.Now().Add(90 * time.Second).UTC().Format(time.RFC3339)
	later := time.Now().Add(600 * time.Second).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":"1","question":"Ethereum Up or Down?","conditionId":"0xETH","slug":"eth","endDate":"` + soon + `","active":true,"closed":false,"clobTokenIds":"[\"e1\",\"e2\"]","outcomes":"[\"Up\",\"Down\"]"},
			{"id":"2","question":"Bitcoin Up or Down?","conditionId":"0xLATER","slug":"btc-later","endDate":"` + later + `","active":true,"closed":false,"clobTokenIds":"[\"b3\",\"b4\"]","outcomes":"[\"Up\",\"Down\"]"},
			{"id":"3","question":"Bitcoin Up or Down?","conditionId":"0xSOON","slug":"btc-soon","endDate":"` + soon + `","active":true,"closed":false,"clobTokenIds":"[\"b1\",\"b2\"]","outcomes":"[\"Up\",\"Down\"]"}
		]`))
	}))
	defer srv.Close()

	c := api.New(api.Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL})
	d := NewDiscoverer(c, config.MarketConfig{Keywords: []string{"bitcoin", "up or down"}})

	m, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if m.ConditionID != "0xSOON" {
		t.Errorf("discovered %s, want 0xSOON (nearest matching BTC market)", m.ConditionID)
	}
}

func TestDiscoverNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"1","question":"Ethereum Up or Down?","conditionId":"0xETH","endDate":"2030-01-01T00:00:00Z","active":true,"closed":false,"clobTokenIds":"[\"e1\",\"e2\"]","outcomes":"[\"Up\",\"Down\"]"}]`))
	}))
	defer srv.Close()

	c := api.New(api.Options{CLOBBaseURL: srv.URL, GammaBaseURL: srv.URL})
	d := NewDiscoverer(c, config.MarketConfig{Keywords: []string{"bitcoin", "up or down"}})
	if _, err := d.Discover(context.Background()); err == nil {
		t.Error("expected error when no market matches")
	}
}
