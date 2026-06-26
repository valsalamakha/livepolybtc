# livepolybtc — Polymarket BTC 5-Minute Up/Down Trading Bot

A production-quality, fully configurable automated trading bot written in **Go**
that trades Polymarket's **Bitcoin 5 Minute UP OR DOWN** markets.

The bot attempts to exploit **late price overreactions** in the final seconds
before a market resolves: when one side spikes to 90–99¢ on aggressive
order-flow while the underlying BTC spot price has barely moved, the move is
likely an overreaction. The bot fades it by buying the cheap opposite side,
aiming to capture the reversal/settlement — and only when the expected payoff
is at least ~4.5× the premium paid.

> ⚠️ **Risk disclaimer.** This is research software for an inherently risky
> strategy on a real-money venue. It ships in **paper/simulation mode by
> default** (`dry_run: true`). Live order submission additionally requires a
> wallet key and the EIP-712 order signer (see [Live trading](#live-trading)).
> Trade only with funds you can afford to lose, and validate thoroughly via
> backtesting and paper trading first.

---

## Table of contents

- [Strategy](#strategy)
- [Features](#features)
- [Project layout](#project-layout)
- [Requirements](#requirements)
- [Setup](#setup)
- [Build & run](#build--run)
- [Configuration guide](#configuration-guide)
- [Authentication](#authentication)
- [Market discovery](#market-discovery)
- [Backtesting](#backtesting)
- [Optimization](#optimization)
- [Logging](#logging)
- [Risk controls](#risk-controls)
- [Concurrency model](#concurrency-model)
- [Live trading](#live-trading)
- [Docker](#docker)
- [Testing](#testing)
- [Architecture & diagrams](#architecture--diagrams)
- [Performance](#performance)

---

## Strategy

The hypothesis, captured in `internal/strategy`:

1. In the final minute one side frequently reaches 90–99¢.
2. In the final ~20 seconds, traders sometimes pile aggressively into one side.
3. Some of these moves **overshoot fair value**.
4. When a statistically abnormal price spike occurs **while BTC movement stays
   inside a configurable threshold**, the bot immediately enters the **opposite**
   side.
5. The trade aims to capture a rapid reversal before settlement.
6. Entries fire **only** when the expected payoff is ≥ `minimum_expected_return`
   (default 4.5×) relative to the premium paid.

**Worked example.** YES (Up) spikes `0.85 → 0.97` in ~5s while BTC moves < $15.
The bot fades by buying NO (Down) at ~3–5¢. Max payoff is $1/share, so the
payoff multiple is `1 / 0.05 = 20×`, comfortably above the 4.5× floor.

Every gate is configurable (see [Configuration guide](#configuration-guide)):
final observation window, minimum odds, maximum BTC move, minimum price spike,
order-book imbalance, minimum liquidity, minimum expected return, maximum
spread, maximum entry price, entry/exit deadlines, and depth levels.

---

## Features

- **Automatic market discovery** — finds the live BTC 5-minute up/down market
  via the Gamma API by keyword/series; **no hard-coded token IDs**.
- **Continuous market data** — order book, best bid/ask, last trade, volume,
  recent trades, implied probabilities, spread, depth, imbalance, timestamps;
  via **websocket** with a **REST polling backstop** and auto-reconnect.
- **External BTC feed** — Coinbase / Binance / Kraken (configurable), with
  price velocity and 5s/20s volatility & move statistics.
- **Configurable strategy engine** — sub-100ms evaluation cadence.
- **Position sizing** — fixed, percent-of-bankroll, or (fractional) Kelly.
- **Execution engine** — marketable limit orders, retries with backoff,
  timeout handling, partial-fill tracking, slippage estimation, cancellation;
  fully **simulated** path for paper trading and backtests.
- **Risk framework** — daily stop loss, max loss per trade, max consecutive
  losses, max simultaneous positions, max daily trades, max exposure, kill
  switch, emergency liquidation hook.
- **Structured logging** — every event to **CSV + JSON** (timestamp, market,
  BTC price, bid/ask, signal strength, entry reason, order id, fill price,
  profit, latency).
- **Backtesting engine** — replay CSV data or generate synthetic datasets;
  reports Sharpe, win rate, profit factor, max drawdown, average return,
  expectancy.
- **Parameter optimizer** — grid search over the strategy parameters.
- **Operational hardening** — heartbeat monitoring, API/websocket reconnect,
  graceful shutdown via context cancellation, panic isolation per goroutine.

---

## Project layout

```
cmd/
  bot/          # main trading bot entry point
  backtest/     # backtesting CLI (+ dataset export)
  optimize/     # parameter optimization CLI
internal/
  api/          # Polymarket CLOB + Gamma REST clients
  app/          # orchestration: wires components into goroutines/channels
  auth/         # CLOB L2 (HMAC) request signing
  backtest/     # historical replay engine, metrics, synthetic data
  btcfeed/      # external BTC price feed + statistics
  config/       # YAML config schema, defaults, validation
  execution/    # order execution engine + simulation + order signer
  logging/      # leveled logger + CSV/JSON event recorder
  marketdata/   # market discovery + websocket/polling collector
  models/       # shared domain types
  optimize/     # grid-search optimization
  risk/         # risk controls + position sizing
configs/        # example config.yaml
docs/           # ARCHITECTURE.md, PERFORMANCE.md
logs/           # event logs (gitignored)
testdata/       # sample backtest dataset
```

---

## Requirements

- **Go 1.24+**
- Outbound HTTPS access to `clob.polymarket.com`, `gamma-api.polymarket.com`,
  the CLOB websocket host, and your chosen BTC exchange.
- (Live trading only) Polymarket CLOB API credentials and a funded wallet.

Dependencies are minimal: `gopkg.in/yaml.v3` and `github.com/gorilla/websocket`.

---

## Setup

```bash
# 1. Clone and enter the repo
git clone https://github.com/valsalamakha/livepolybtc && cd livepolybtc

# 2. (Live only) provide credentials
cp .env.example .env
# edit .env and export the variables, or source it:
set -a && . ./.env && set +a

# 3. Review/adjust the configuration
$EDITOR configs/config.yaml
```

### Environment variables

| Variable                  | Required        | Purpose                                              |
|---------------------------|-----------------|------------------------------------------------------|
| `POLYMARKET_API_KEY`      | live only       | CLOB L2 API key                                      |
| `POLYMARKET_SECRET`       | live only       | CLOB L2 secret (base64url) for HMAC signing          |
| `POLYMARKET_PASSPHRASE`   | live only       | CLOB L2 passphrase                                   |
| `POLYMARKET_ADDRESS`      | optional        | Maker/funder wallet address (`POLY_ADDRESS` header)  |
| `POLYMARKET_PRIVATE_KEY`  | live orders     | Wallet key for EIP-712 order signing                 |
| `POLYMARKET_DRY_RUN`      | optional        | `true`/`1` forces simulation mode                    |

Credentials are **never** read from the YAML file or committed to disk.

---

## Build & run

Using the Makefile (`make help` lists everything):

```bash
make build           # build bot, backtest, optimize into ./bin
make test            # run unit + integration tests
make test-race       # tests with the race detector
make paper           # run the bot in paper/simulation mode (safe)
make backtest-run    # backtest on synthetic data
make optimize-run    # grid-search the parameters
```

Or directly:

```bash
go run ./cmd/bot -config configs/config.yaml -dry-run   # paper mode
go run ./cmd/bot -config configs/config.yaml -live       # LIVE (needs creds)
```

Flags: `-config <path>`, `-dry-run` (force simulation), `-live` (enable live
trading). Stop with `Ctrl-C` — the bot drains and shuts down gracefully.

---

## Configuration guide

All parameters live in `configs/config.yaml` and require **no recompilation** to
change. Key sections:

| Section      | Highlights                                                                 |
|--------------|----------------------------------------------------------------------------|
| `polymarket` | REST/Gamma/WS endpoints, chain id                                          |
| `btc_feed`   | `exchange` (coinbase/binance/kraken), `symbol`, poll interval, staleness   |
| `market`     | discovery `keywords`, optional `series_slug`, discovery interval           |
| `strategy`   | the optimization targets — windows, odds, BTC move, spike, imbalance, etc. |
| `sizing`     | `mode` (fixed/percent/kelly), bankroll, sizes, Kelly params, caps          |
| `execution`  | order type, marketable buffer, retries/backoff, timeout, tick size         |
| `risk`       | daily stop, per-trade loss, consecutive losses, positions, exposure, kill  |
| `logging`    | dir, level, csv/json/console toggles                                       |
| `health`     | heartbeat interval, reconnect backoff/limits                               |

Strategy parameters (the main optimization knobs):

```yaml
strategy:
  final_observation_window: 20s     # only evaluate within this of resolution
  spike_lookback: 5s                # window for measuring a spike
  minimum_odds: 0.90                # overreacted side must reach this price
  maximum_btc_move: 15.0            # max BTC move (USD) over lookback to qualify
  minimum_price_spike: 0.08         # min rise (cents) of the overreacted side
  minimum_order_book_imbalance: 0.50
  minimum_liquidity: 50.0
  minimum_expected_return: 4.5      # required payoff multiple (1/entry_price)
  maximum_spread: 0.05
  maximum_entry_price: 0            # 0 => derived = 1/minimum_expected_return
  entry_deadline: 3s                # do not enter within this of resolution
  exit_deadline: 2s
  depth_levels: 5
```

`maximum_entry_price` defaults to `1 / minimum_expected_return` when set to 0.

---

## Authentication

Polymarket's CLOB uses **L2 (API-key) authentication** for private endpoints.
`internal/auth` signs each request with:

```
signature = base64url( HMAC_SHA256( base64url_decode(secret),
                                    timestamp + METHOD + path + body ) )
```

and sends the `POLY_SIGNATURE`, `POLY_TIMESTAMP`, `POLY_API_KEY`,
`POLY_PASSPHRASE` and (optionally) `POLY_ADDRESS` headers. See the
[Polymarket API reference](https://docs.polymarket.us/api-reference).

---

## Market discovery

`internal/marketdata.Discoverer` queries the Gamma `/markets` endpoint for
active, non-closed markets, filters by the configured `keywords` (and optional
`series_slug`), requires exactly two outcome tokens, and selects the one
resolving **soonest in the future** — i.e. the live 5-minute window. Token IDs
are resolved dynamically; nothing is hard-coded. The market rolls over
automatically as each window resolves.

---

## Backtesting

```bash
# Synthetic dataset (no data needed)
go run ./cmd/backtest -synthetic 300 -seed 42 -trades

# Your own CSV dataset
go run ./cmd/backtest -data testdata/sample.csv

# Export a dataset (e.g. synthetic) to the canonical CSV schema
go run ./cmd/backtest -synthetic 50 -export mydata.csv
```

Reported metrics: **win rate, net P&L, profit factor, average return,
expectancy, (per-trade) Sharpe ratio, and max drawdown**.

**CSV schema** (see `testdata/sample.csv`):

```
timestamp,condition_id,end_time,up_token,down_token,
up_bid,up_bid_size,up_ask,up_ask_size,
down_bid,down_bid_size,down_ask,down_ask_size,btc_price
```

Timestamps may be RFC3339 or unix seconds. Outcomes are inferred from the final
tick of each market (the side with mid ≥ 0.5 wins).

---

## Optimization

```bash
go run ./cmd/optimize -synthetic 300 -objective sharpe -top 10
```

Grid-searches over BTC move threshold, price spike, observation window,
order-book imbalance and expected payout (extend the grid in
`cmd/optimize/main.go` or `internal/optimize`). Objectives: `netpl`, `sharpe`,
`expectancy`. Results are ranked best-first.

---

## Logging

`internal/logging` writes timestamped `events-<stamp>.csv` and
`events-<stamp>.jsonl` into `logging.dir` (default `logs/`). Each row captures:
timestamp, type, market, BTC price, best bid/ask, signal strength, entry
reason, order id, side, price, size, fill price, profit, latency (ms), message.
Console logging is leveled (`debug`/`info`/`warn`/`error`).

---

## Risk controls

`internal/risk.Manager` is the single source of truth for tradeability and
enforces: daily stop loss (auto-trips the kill switch), max loss per trade,
max consecutive losses, max simultaneous positions, max daily trades, and max
total exposure. It also tracks bankroll and exposure, supports a manual kill
switch/reset, and rolls daily counters at UTC midnight. The orchestrator adds
heartbeat monitoring, API/websocket reconnection, and graceful shutdown (with
`cancel-all` and an emergency-liquidation hook in live mode).

---

## Concurrency model

The bot runs cooperating goroutines that communicate over channels and shared,
mutex-guarded state, all cancelled by a single `context.Context`:

- **btc-feed** — polls the exchange, maintains price stats.
- **market-lifecycle** — discovers the active market and drives collection for
  its lifetime, then rolls over.
- **strategy** — evaluates snapshots every 50ms and emits signals.
- **execution** — sizes, gates (risk) and executes signals.
- **settlement** — resolves positions post-expiry and updates risk accounting.
- **health** — heartbeats and watches feed/connection health.

Panics in any goroutine are isolated and trip the kill switch.

---

## Live trading

By default the bot **simulates** fills against the live order book (paper
trading) so it is safe to run against production market data. Submitting **real**
orders to Polymarket additionally requires **EIP-712 order signing** with a
wallet private key. That signer is intentionally pluggable
(`execution.OrderSigner`); the default `unsignedSigner` reports itself
unavailable, keeping the bot in simulation mode. To go live:

1. Provide `POLYMARKET_API_KEY` / `SECRET` / `PASSPHRASE` (and `ADDRESS`).
2. Implement `execution.OrderSigner` using your wallet key + an Ethereum
   signing library (e.g. `go-ethereum`) and wire it in `app.New`.
3. Run with `-live`.

This separation keeps the core dependency-light and prevents accidental live
trading.

---

## Docker

```bash
docker build -t livepolybtc .
docker compose up --build      # uses .env for credentials; paper mode by default
```

`docker-compose.yml` mounts `./logs` and `./configs`, reads credentials from
`.env`, and defaults to paper mode. Switch the `command` to `-live` to trade.

---

## Testing

```bash
make test        # unit + integration tests
make test-race   # with the race detector
make cover       # HTML coverage report
```

Coverage includes: config load/validate, order-book math, HMAC signing,
strategy entry logic (fires/does-not-fire across every gate), risk limits and
sizing, execution simulation/partial fills, backtest metrics, a **mock REST
API** (httptest), a **mock websocket** server (gorilla), the BTC feed
statistics, and a full bot orchestration integration test.

---

## Architecture & diagrams

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the component and
sequence diagrams.

## Performance

See [`docs/PERFORMANCE.md`](docs/PERFORMANCE.md) for latency targets and
optimization recommendations (sub-100ms signal processing, minimal allocations,
context cancellation, reconnection).
