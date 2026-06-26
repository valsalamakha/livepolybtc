package config

import "time"

// Default returns a Config populated with sensible production defaults. It is
// also the base onto which YAML values are overlaid.
func Default() *Config {
	return &Config{
		DryRun: true,
		Polymarket: PolymarketConfig{
			RestBaseURL:  "https://clob.polymarket.com",
			GammaBaseURL: "https://gamma-api.polymarket.com",
			WSBaseURL:    "wss://ws-subscriptions-clob.polymarket.com/ws",
			ChainID:      137,
		},
		BTCFeed: BTCFeedConfig{
			Exchange:      "coinbase",
			Symbol:        "BTC-USD",
			PollInterval:  500 * time.Millisecond,
			UseWebsocket:  true,
			StaleAfter:    3 * time.Second,
			HistoryWindow: 60 * time.Second,
		},
		Market: MarketConfig{
			Keywords:          []string{"bitcoin", "up or down"},
			DiscoveryInterval: 15 * time.Second,
			UpOutcome:         "Up",
			DownOutcome:       "Down",
		},
		Strategy: StrategyConfig{
			FinalObservationWindow:    20 * time.Second,
			SpikeLookback:             5 * time.Second,
			MinimumOdds:               0.90,
			MaximumBTCMove:            15.0,
			MinimumPriceSpike:         0.08,
			MinimumOrderBookImbalance: 0.50,
			MinimumLiquidity:          50.0,
			MinimumExpectedReturn:     4.5,
			MaximumSpread:             0.05,
			MaximumEntryPrice:         0, // derived from MinimumExpectedReturn
			EntryDeadline:             3 * time.Second,
			ExitDeadline:              2 * time.Second,
			MaxBTCStaleness:           2 * time.Second,
			DepthLevels:               5,
		},
		Sizing: SizingConfig{
			Mode:              SizeFixed,
			Bankroll:          1000.0,
			FixedSize:         20.0,
			PercentOfBankroll: 0.02,
			KellyFraction:     0.25,
			WinProbability:    0.55,
			MaxTradeSize:      50.0,
			MinTradeSize:      1.0,
		},
		Execution: ExecutionConfig{
			OrderType:             "FAK",
			MarketableLimitBuffer: 0.01,
			MaxSlippage:           0.02,
			MaxRetries:            3,
			RetryBackoff:          200 * time.Millisecond,
			OrderTimeout:          2 * time.Second,
			TickSize:              0.01,
		},
		Risk: RiskConfig{
			DailyStopLoss:            100.0,
			MaxLossPerTrade:          25.0,
			MaxConsecutiveLosses:     5,
			MaxSimultaneousPositions: 1,
			MaxDailyTrades:           50,
			MaxExposure:              100.0,
			KillSwitchEnabled:        true,
			EmergencyLiquidate:       true,
		},
		Logging: LoggingConfig{
			Dir:     "logs",
			Level:   "info",
			CSV:     true,
			JSON:    true,
			Console: true,
		},
		Health: HealthConfig{
			HeartbeatInterval:    5 * time.Second,
			MaxReconnectAttempts: 0,
			ReconnectBackoff:     time.Second,
			MaxReconnectBackoff:  30 * time.Second,
		},
	}
}
