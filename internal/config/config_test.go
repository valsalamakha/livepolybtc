package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	cfg := Default()
	cfg.ApplyDerived()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestDerivedMaximumEntryPrice(t *testing.T) {
	cfg := Default()
	cfg.Strategy.MinimumExpectedReturn = 5.0
	cfg.Strategy.MaximumEntryPrice = 0
	cfg.ApplyDerived()
	if got := cfg.Strategy.MaximumEntryPrice; got != 0.2 {
		t.Errorf("derived max entry price = %v, want 0.2", got)
	}
}

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	yaml := `
dry_run: true
strategy:
  minimum_odds: 0.92
  minimum_expected_return: 5.0
  final_observation_window: 25s
sizing:
  mode: percent
  bankroll: 5000
  percent_of_bankroll: 0.03
btc_feed:
  exchange: binance
  symbol: BTCUSDT
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Strategy.MinimumOdds != 0.92 {
		t.Errorf("minimum_odds = %v, want 0.92", cfg.Strategy.MinimumOdds)
	}
	if cfg.Strategy.MaximumEntryPrice != 0.2 {
		t.Errorf("derived max entry = %v, want 0.2", cfg.Strategy.MaximumEntryPrice)
	}
	if cfg.Sizing.Mode != SizePercent {
		t.Errorf("mode = %v, want percent", cfg.Sizing.Mode)
	}
	if cfg.BTCFeed.Exchange != "binance" {
		t.Errorf("exchange = %v, want binance", cfg.BTCFeed.Exchange)
	}
}

func TestEnvOverridesCredentials(t *testing.T) {
	t.Setenv("POLYMARKET_API_KEY", "key123")
	t.Setenv("POLYMARKET_SECRET", "sec123")
	t.Setenv("POLYMARKET_PASSPHRASE", "pass123")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Polymarket.APIKey != "key123" {
		t.Errorf("api key not loaded from env")
	}
	if err := cfg.RequireCredentials(); err != nil {
		t.Errorf("RequireCredentials should pass: %v", err)
	}
}

func TestRequireCredentialsMissing(t *testing.T) {
	cfg := Default()
	if err := cfg.RequireCredentials(); err == nil {
		t.Error("expected missing-credentials error")
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cfg := Default()
	cfg.Strategy.MinimumOdds = 1.5
	cfg.ApplyDerived()
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for minimum_odds > 1")
	}
}
