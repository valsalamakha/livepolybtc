# ---- build stage ----
FROM golang:1.24-alpine AS build

WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Build the bot (static binary).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/livepolybtc ./cmd/bot \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/backtest ./cmd/backtest \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/optimize ./cmd/optimize

# ---- runtime stage ----
FROM alpine:3.20

# TLS roots for outbound HTTPS to Polymarket / exchanges.
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 bot

WORKDIR /app
COPY --from=build /out/livepolybtc /usr/local/bin/livepolybtc
COPY --from=build /out/backtest /usr/local/bin/backtest
COPY --from=build /out/optimize /usr/local/bin/optimize
COPY configs/ /app/configs/

# Logs are written here; mount a volume to persist them.
RUN mkdir -p /app/logs && chown -R bot:bot /app
USER bot

ENTRYPOINT ["livepolybtc"]
CMD ["-config", "/app/configs/config.yaml", "-dry-run"]
