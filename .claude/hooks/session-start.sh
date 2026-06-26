#!/bin/bash
# SessionStart hook: prepare the Go toolchain caches so tests, `go vet` and
# gofmt work immediately in Claude Code on the web sessions.
set -euo pipefail

# Only run in the remote (Claude Code on the web) environment.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "$CLAUDE_PROJECT_DIR"

if ! command -v go >/dev/null 2>&1; then
  echo "session-start: go toolchain not found on PATH" >&2
  exit 1
fi

echo "session-start: downloading Go module dependencies..."
go mod download

# Warm the build/vet caches (cached in the container) so the first test or
# lint run in the session is fast. Non-fatal: a transient failure here must
# not block the session from starting.
echo "session-start: warming build cache..."
go build ./... || true
go vet ./... || true

echo "session-start: ready."
