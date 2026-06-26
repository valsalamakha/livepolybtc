# Makefile for the Polymarket BTC 5-minute up/down trading bot.

BINARY      := livepolybtc
PKG         := github.com/valsalamakha/livepolybtc
BIN_DIR     := bin
CONFIG      ?= configs/config.yaml
GO          ?= go
GOFLAGS     ?=

.PHONY: all build bot backtest optimize test test-race cover vet fmt fmt-check tidy lint run paper backtest-run optimize-run clean docker docker-compose-up help

all: fmt-check vet test build ## Format-check, vet, test and build everything

build: bot backtest optimize ## Build all binaries

bot: ## Build the trading bot
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/bot

backtest: ## Build the backtester CLI
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/backtest ./cmd/backtest

optimize: ## Build the optimizer CLI
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/optimize ./cmd/optimize

test: ## Run unit + integration tests
	$(GO) test ./...

test-race: ## Run tests with the race detector
	$(GO) test -race ./...

cover: ## Run tests and produce an HTML coverage report
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "coverage report written to coverage.html"

vet: ## Run go vet
	$(GO) vet ./...

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-clean
	@test -z "$$(gofmt -l .)" || (echo "unformatted files:"; gofmt -l .; exit 1)

tidy: ## Tidy go.mod/go.sum
	$(GO) mod tidy

run: paper ## Alias for paper (safe) mode

paper: bot ## Run the bot in simulation/paper mode (no live orders)
	./$(BIN_DIR)/$(BINARY) -config $(CONFIG) -dry-run

live: bot ## Run the bot in LIVE mode (requires credentials)
	./$(BIN_DIR)/$(BINARY) -config $(CONFIG) -live

backtest-run: backtest ## Run a backtest on the synthetic dataset
	./$(BIN_DIR)/backtest -config $(CONFIG) -synthetic 300 -trades

optimize-run: optimize ## Run the parameter optimizer on the synthetic dataset
	./$(BIN_DIR)/optimize -config $(CONFIG) -objective netpl -top 10

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) coverage.out coverage.html

docker: ## Build the Docker image
	docker build -t $(BINARY):latest .

docker-compose-up: ## Start via docker-compose
	docker compose up --build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
