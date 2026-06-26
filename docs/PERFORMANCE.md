# Performance notes & optimization recommendations

The strategy edge lives in the last ~20 seconds before resolution, so the
critical path is **market-data update → signal → order**. The targets and the
recommendations below keep that path tight.

## Targets

| Metric                        | Target            | Where enforced                              |
|-------------------------------|-------------------|---------------------------------------------|
| Signal evaluation latency     | **< 100 ms**      | 50 ms strategy tick; O(levels) book math    |
| Snapshot build                | < 1 ms            | shallow clones of small books               |
| Order submission round-trip   | network-bound     | FAK marketable limit, retries with backoff  |
| Allocations per evaluation    | minimal           | reused maps/slices, value types in hot path |

## What the implementation already does

- **Fast evaluation cadence.** The strategy goroutine ticks every 50 ms and the
  per-evaluation work is O(book depth) — best bid/ask, spread, imbalance and
  liquidity are simple linear scans over a handful of levels.
- **Websocket-first market data** with a REST polling backstop so the latest
  book is in memory; the strategy never blocks on I/O.
- **Channel decoupling.** Signals flow to execution over a buffered channel; a
  full queue drops the oldest intent rather than stalling evaluation.
- **Value types in the hot path.** `BTCStats`, `Snapshot`, `PriceLevel` are
  small value/`struct` types; books are cloned shallowly only when handed across
  goroutines.
- **Context cancellation everywhere.** Every loop and HTTP call honours the root
  context for sub-second graceful shutdown.
- **Bounded histories.** Price histories evict by time window, so memory is
  flat regardless of run length.

## Recommendations for going faster / further

1. **Co-locate near the venue.** Latency to `clob.polymarket.com` dominates the
   order round-trip. Run in a region close to Polymarket's infrastructure.
2. **Pre-sign / pre-stage orders.** In live mode, build and EIP-712-sign a small
   set of candidate orders (per token, per size bucket) *before* the final
   window so submission is a single HTTP POST when the signal fires.
3. **Pin the strategy goroutine & reduce GC pressure.** For ultra-low latency,
   set `GOMAXPROCS` deliberately, raise `GOGC` (or use a `debug.SetGCPercent`
   tune / `GOMEMLIMIT`) to avoid GC pauses during the critical 20 s, and reuse
   buffers via `sync.Pool` if profiling shows allocation hotspots.
4. **Tighten the tick adaptively.** Outside the observation window, slow the
   strategy tick (e.g. 250 ms) and speed it to 10–20 ms only inside the final
   window to cut idle wake-ups while maximizing in-window resolution.
5. **Use the user (authenticated) websocket** for fills instead of inferring
   from the order response, to get exact fill prices/sizes with lower latency.
6. **Batch book math.** If subscribing to many markets, precompute imbalance and
   depth incrementally on each `price_change` delta rather than re-scanning.
7. **Profile with `pprof`.** Add `net/http/pprof` behind a flag and capture CPU
   profiles during paper runs to find real hotspots before micro-optimizing.
8. **Measure, don't guess.** The event log records per-order latency
   (`latency_ms`); track its distribution and alert if the p99 of
   market-data-to-order exceeds your budget.

## Benchmarking

Run the backtester to validate strategy parameters offline (no network), and
the optimizer to sweep them:

```bash
go run ./cmd/backtest -synthetic 1000 -seed 1
go run ./cmd/optimize  -synthetic 1000 -objective sharpe -top 10
```

For micro-benchmarks of the hot path, add Go benchmarks alongside the
`strategy` and `models` tests and run `go test -bench=. -benchmem ./...`.
