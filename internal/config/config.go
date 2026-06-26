// Package config defines the bot's configuration schema and loading logic.
//
// Configuration is layered: defaults are applied first, then values from a
// YAML file, then a small number of secrets sourced from environment
// variables. No parameter requires recompilation to change.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration object.
type Config struct {
	Polymarket PolymarketConfig `yaml:"polymarket"`
	BTCFeed    BTCFeedConfig    `yaml:"btc_feed"`
	Market     MarketConfig     `yaml:"market"`
	Strategy   StrategyConfig   `yaml:"strategy"`
	Sizing     SizingConfig     `yaml:"sizing"`
	Execution  ExecutionConfig  `yaml:"execution"`
	Risk       RiskConfig       `yaml:"risk"`
	Logging    LoggingConfig    `yaml:"logging"`
	Health     HealthConfig     `yaml:"health"`
	DryRun     bool             `yaml:"dry_run"`
}

// PolymarketConfig holds API endpoints and credentials. Credentials are loaded
// from the environment, never from disk.
type PolymarketConfig struct {
	RestBaseURL  string `yaml:"rest_base_url"`
	GammaBaseURL string `yaml:"gamma_base_url"`
	WSBaseURL    string `yaml:"ws_base_url"`
	ChainID      int    `yaml:"chain_id"`

	// Credentials are populated from environment variables by Load and are not
	// read from the YAML file.
	APIKey     string `yaml:"-"`
	Secret     string `yaml:"-"`
	Passphrase string `yaml:"-"`
	// Address is the funder/maker wallet address (optional, env-sourced).
	Address string `yaml:"-"`
	// PrivateKey is the wallet key used for EIP-712 order signing (optional).
	PrivateKey string `yaml:"-"`
}

// BTCFeedConfig configures the external Bitcoin spot price feed.
type BTCFeedConfig struct {
	// Exchange selects the price source: "coinbase", "binance", or "kraken".
	Exchange string `yaml:"exchange"`
	// Symbol is the trading pair on that exchange (e.g. "BTC-USD").
	Symbol string `yaml:"symbol"`
	// PollInterval is how often to sample the price when using REST polling.
	PollInterval time.Duration `yaml:"poll_interval"`
	// UseWebsocket prefers a streaming feed when the exchange supports it.
	UseWebsocket bool `yaml:"use_websocket"`
	// StaleAfter marks the feed stale if no update arrives within this window.
	StaleAfter time.Duration `yaml:"stale_after"`
	// HistoryWindow is how much price history to retain for volatility calc.
	HistoryWindow time.Duration `yaml:"history_window"`
}

// MarketConfig configures automatic market discovery.
type MarketConfig struct {
	// Keywords that must all appear in the market question to match (case
	// insensitive). Defaults target the BTC 5-minute up/down market.
	Keywords []string `yaml:"keywords"`
	// SeriesSlug optionally narrows discovery to a known series slug.
	SeriesSlug string `yaml:"series_slug"`
	// DiscoveryInterval is how often to refresh the active market.
	DiscoveryInterval time.Duration `yaml:"discovery_interval"`
	// UpOutcome / DownOutcome are the expected outcome labels.
	UpOutcome   string `yaml:"up_outcome"`
	DownOutcome string `yaml:"down_outcome"`
}

// StrategyConfig holds the fully configurable entry parameters. These are the
// primary targets of later optimization.
type StrategyConfig struct {
	// FinalObservationWindow: only evaluate entries within this much time of
	// resolution.
	FinalObservationWindow time.Duration `yaml:"final_observation_window"`
	// SpikeLookback is the window over which an abnormal price spike is
	// measured.
	SpikeLookback time.Duration `yaml:"spike_lookback"`
	// MinimumOdds: the overreacted side must trade at or above this price.
	MinimumOdds float64 `yaml:"minimum_odds"`
	// MaximumBTCMove: the absolute BTC move (USD) over the spike lookback must
	// stay at or below this to qualify as an overreaction.
	MaximumBTCMove float64 `yaml:"maximum_btc_move"`
	// MinimumPriceSpike: minimum increase (in cents, e.g. 0.08 == 8c) of the
	// overreacted side over the spike lookback.
	MinimumPriceSpike float64 `yaml:"minimum_price_spike"`
	// MinimumOrderBookImbalance: required imbalance magnitude [0,1] on the
	// overreacted side.
	MinimumOrderBookImbalance float64 `yaml:"minimum_order_book_imbalance"`
	// MinimumLiquidity: required resting notional on the entry side.
	MinimumLiquidity float64 `yaml:"minimum_liquidity"`
	// MinimumExpectedReturn: required payoff multiple (1/entry_price).
	MinimumExpectedReturn float64 `yaml:"minimum_expected_return"`
	// MaximumSpread: reject when the entry book spread exceeds this.
	MaximumSpread float64 `yaml:"maximum_spread"`
	// MaximumEntryPrice: never pay more than this for the faded side. Derived
	// from MinimumExpectedReturn when zero.
	MaximumEntryPrice float64 `yaml:"maximum_entry_price"`
	// EntryDeadline: do not enter within this much time of resolution (too
	// late to fill / settle).
	EntryDeadline time.Duration `yaml:"entry_deadline"`
	// ExitDeadline: attempt to flatten any open position within this much time
	// of resolution.
	ExitDeadline time.Duration `yaml:"exit_deadline"`
	// MaxBTCStaleness rejects signals when the BTC feed is older than this.
	MaxBTCStaleness time.Duration `yaml:"max_btc_staleness"`
	// DepthLevels is how many book levels to include in imbalance/liquidity.
	DepthLevels int `yaml:"depth_levels"`
}

// SizingMode selects the position-sizing algorithm.
type SizingMode string

const (
	// SizeFixed uses a fixed dollar notional per trade.
	SizeFixed SizingMode = "fixed"
	// SizePercent risks a fixed percentage of bankroll per trade.
	SizePercent SizingMode = "percent"
	// SizeKelly uses a (capped) Kelly fraction.
	SizeKelly SizingMode = "kelly"
)

// SizingConfig configures position sizing.
type SizingConfig struct {
	Mode SizingMode `yaml:"mode"`
	// Bankroll is the starting bankroll in USDC.
	Bankroll float64 `yaml:"bankroll"`
	// FixedSize is the dollar notional per trade when Mode == fixed.
	FixedSize float64 `yaml:"fixed_size"`
	// PercentOfBankroll is the fraction (0-1) risked when Mode == percent.
	PercentOfBankroll float64 `yaml:"percent_of_bankroll"`
	// KellyFraction scales the computed Kelly stake (0-1) when Mode == kelly.
	KellyFraction float64 `yaml:"kelly_fraction"`
	// WinProbability is the assumed edge used by the Kelly calculation.
	WinProbability float64 `yaml:"win_probability"`
	// MaxTradeSize caps the per-trade notional regardless of mode.
	MaxTradeSize float64 `yaml:"max_trade_size"`
	// MinTradeSize is the minimum viable per-trade notional.
	MinTradeSize float64 `yaml:"min_trade_size"`
}

// ExecutionConfig configures the order execution engine.
type ExecutionConfig struct {
	// OrderType selects the default order type ("FOK", "FAK", "GTC").
	OrderType string `yaml:"order_type"`
	// MarketableLimitBuffer is added to the best ask when crossing the spread
	// with a marketable limit order.
	MarketableLimitBuffer float64 `yaml:"marketable_limit_buffer"`
	// MaxSlippage caps the acceptable slippage vs the signal price.
	MaxSlippage float64 `yaml:"max_slippage"`
	// MaxRetries is the number of submission retries on transient failure.
	MaxRetries int `yaml:"max_retries"`
	// RetryBackoff is the base backoff between retries.
	RetryBackoff time.Duration `yaml:"retry_backoff"`
	// OrderTimeout is how long to wait for an order to fill before cancelling.
	OrderTimeout time.Duration `yaml:"order_timeout"`
	// TickSize is the price increment used to round limit prices.
	TickSize float64 `yaml:"tick_size"`
}

// RiskConfig configures the risk-control framework.
type RiskConfig struct {
	// DailyStopLoss is the maximum daily loss (USD) before trading halts.
	DailyStopLoss float64 `yaml:"daily_stop_loss"`
	// MaxLossPerTrade caps the loss tolerated on a single trade (USD).
	MaxLossPerTrade float64 `yaml:"max_loss_per_trade"`
	// MaxConsecutiveLosses halts trading after this many losses in a row.
	MaxConsecutiveLosses int `yaml:"max_consecutive_losses"`
	// MaxSimultaneousPositions caps concurrently open positions.
	MaxSimultaneousPositions int `yaml:"max_simultaneous_positions"`
	// MaxDailyTrades caps the number of entries per day.
	MaxDailyTrades int `yaml:"max_daily_trades"`
	// MaxExposure caps total open notional (USD).
	MaxExposure float64 `yaml:"max_exposure"`
	// KillSwitchEnabled enables the manual/automatic kill switch.
	KillSwitchEnabled bool `yaml:"kill_switch_enabled"`
	// EmergencyLiquidate attempts to flatten positions when the kill switch
	// fires (where supported).
	EmergencyLiquidate bool `yaml:"emergency_liquidate"`
}

// LoggingConfig configures structured event logging.
type LoggingConfig struct {
	// Dir is the directory where log files are written.
	Dir string `yaml:"dir"`
	// Level is the minimum log level ("debug", "info", "warn", "error").
	Level string `yaml:"level"`
	// CSV enables the CSV event log.
	CSV bool `yaml:"csv"`
	// JSON enables the JSON-lines event log.
	JSON bool `yaml:"json"`
	// Console enables human-readable console logging.
	Console bool `yaml:"console"`
}

// HealthConfig configures heartbeat and reconnection behavior.
type HealthConfig struct {
	// HeartbeatInterval is how often health is reported.
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	// MaxReconnectAttempts limits websocket/API reconnect attempts (0 = inf).
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`
	// ReconnectBackoff is the base reconnect backoff.
	ReconnectBackoff time.Duration `yaml:"reconnect_backoff"`
	// MaxReconnectBackoff caps the exponential reconnect backoff.
	MaxReconnectBackoff time.Duration `yaml:"max_reconnect_backoff"`
}

// Load reads configuration from the given YAML path (applying defaults for any
// omitted fields), overlays environment-sourced secrets, and validates the
// result. An empty path uses defaults only.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %q: %w", path, err)
		}
	}

	cfg.applyEnv()
	cfg.ApplyDerived()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv overlays secrets and overrides from environment variables.
func (c *Config) applyEnv() {
	c.Polymarket.APIKey = firstNonEmpty(os.Getenv("POLYMARKET_API_KEY"), c.Polymarket.APIKey)
	c.Polymarket.Secret = firstNonEmpty(os.Getenv("POLYMARKET_SECRET"), c.Polymarket.Secret)
	c.Polymarket.Passphrase = firstNonEmpty(os.Getenv("POLYMARKET_PASSPHRASE"), c.Polymarket.Passphrase)
	c.Polymarket.Address = firstNonEmpty(os.Getenv("POLYMARKET_ADDRESS"), c.Polymarket.Address)
	c.Polymarket.PrivateKey = firstNonEmpty(os.Getenv("POLYMARKET_PRIVATE_KEY"), c.Polymarket.PrivateKey)

	if v := os.Getenv("POLYMARKET_DRY_RUN"); v == "true" || v == "1" {
		c.DryRun = true
	}
}

// ApplyDerived fills in parameters that are computed from others when unset. It
// is invoked by Load and is exported so callers constructing a Config in code
// (tests, backtests) can populate derived fields.
func (c *Config) ApplyDerived() {
	if c.Strategy.MaximumEntryPrice <= 0 && c.Strategy.MinimumExpectedReturn > 0 {
		c.Strategy.MaximumEntryPrice = 1.0 / c.Strategy.MinimumExpectedReturn
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
