package config

import (
	"errors"
	"fmt"
)

// Validate checks the configuration for internal consistency. It does not
// require credentials to be present (dry-run / backtesting do not need them);
// credential presence is checked by RequireCredentials.
func (c *Config) Validate() error {
	var errs []error

	if c.Polymarket.RestBaseURL == "" {
		errs = append(errs, errors.New("polymarket.rest_base_url is required"))
	}
	if c.Polymarket.GammaBaseURL == "" {
		errs = append(errs, errors.New("polymarket.gamma_base_url is required"))
	}

	switch c.BTCFeed.Exchange {
	case "coinbase", "binance", "kraken":
	default:
		errs = append(errs, fmt.Errorf("btc_feed.exchange %q is not supported (coinbase|binance|kraken)", c.BTCFeed.Exchange))
	}
	if c.BTCFeed.PollInterval <= 0 {
		errs = append(errs, errors.New("btc_feed.poll_interval must be > 0"))
	}

	if len(c.Market.Keywords) == 0 && c.Market.SeriesSlug == "" {
		errs = append(errs, errors.New("market.keywords or market.series_slug must be set"))
	}

	s := c.Strategy
	if s.FinalObservationWindow <= 0 {
		errs = append(errs, errors.New("strategy.final_observation_window must be > 0"))
	}
	if s.SpikeLookback <= 0 {
		errs = append(errs, errors.New("strategy.spike_lookback must be > 0"))
	}
	if s.MinimumOdds <= 0.5 || s.MinimumOdds >= 1.0 {
		errs = append(errs, errors.New("strategy.minimum_odds must be in (0.5,1.0)"))
	}
	if s.MinimumExpectedReturn < 1.0 {
		errs = append(errs, errors.New("strategy.minimum_expected_return must be >= 1.0"))
	}
	if s.MaximumEntryPrice <= 0 || s.MaximumEntryPrice >= 1.0 {
		errs = append(errs, errors.New("strategy.maximum_entry_price must resolve to (0,1)"))
	}
	if s.MinimumPriceSpike <= 0 {
		errs = append(errs, errors.New("strategy.minimum_price_spike must be > 0"))
	}
	if s.EntryDeadline >= s.FinalObservationWindow {
		errs = append(errs, errors.New("strategy.entry_deadline must be < final_observation_window"))
	}
	if s.DepthLevels < 1 {
		errs = append(errs, errors.New("strategy.depth_levels must be >= 1"))
	}

	switch c.Sizing.Mode {
	case SizeFixed, SizePercent, SizeKelly:
	default:
		errs = append(errs, fmt.Errorf("sizing.mode %q is invalid (fixed|percent|kelly)", c.Sizing.Mode))
	}
	if c.Sizing.Bankroll <= 0 {
		errs = append(errs, errors.New("sizing.bankroll must be > 0"))
	}
	if c.Sizing.MaxTradeSize <= 0 {
		errs = append(errs, errors.New("sizing.max_trade_size must be > 0"))
	}
	if c.Sizing.Mode == SizePercent && (c.Sizing.PercentOfBankroll <= 0 || c.Sizing.PercentOfBankroll > 1) {
		errs = append(errs, errors.New("sizing.percent_of_bankroll must be in (0,1]"))
	}
	if c.Sizing.Mode == SizeKelly && (c.Sizing.KellyFraction <= 0 || c.Sizing.KellyFraction > 1) {
		errs = append(errs, errors.New("sizing.kelly_fraction must be in (0,1]"))
	}

	switch c.Execution.OrderType {
	case "GTC", "GTD", "FOK", "FAK":
	default:
		errs = append(errs, fmt.Errorf("execution.order_type %q is invalid (GTC|GTD|FOK|FAK)", c.Execution.OrderType))
	}
	if c.Execution.TickSize <= 0 {
		errs = append(errs, errors.New("execution.tick_size must be > 0"))
	}
	if c.Execution.MaxRetries < 0 {
		errs = append(errs, errors.New("execution.max_retries must be >= 0"))
	}

	if c.Risk.MaxSimultaneousPositions < 1 {
		errs = append(errs, errors.New("risk.max_simultaneous_positions must be >= 1"))
	}

	return errors.Join(errs...)
}

// RequireCredentials returns an error if any required API credential is
// missing. Call this before live trading (not for dry-run/backtest).
func (c *Config) RequireCredentials() error {
	var missing []string
	if c.Polymarket.APIKey == "" {
		missing = append(missing, "POLYMARKET_API_KEY")
	}
	if c.Polymarket.Secret == "" {
		missing = append(missing, "POLYMARKET_SECRET")
	}
	if c.Polymarket.Passphrase == "" {
		missing = append(missing, "POLYMARKET_PASSPHRASE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required credentials: %v", missing)
	}
	return nil
}
