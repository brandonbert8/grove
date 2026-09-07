#!/usr/bin/env sh
# Smoke test: build everything, run unit tests, boot the hello example
# and assert GET /hello returns the expected JSON.
set -eu
cd "$(dirname "$0")/.."

go build ./...
go test ./...

PORT=18080 go run ./examples/hello &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT INT TERM

for i in $(seq 1 50); do
  if curl -sf http://localhost:18080/hello >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

BODY=$(curl -sf http://localhost:18080/hello)
echo "GET /hello -> $BODY"
echo "$BODY" | grep -q '"Hello Grove"' || {
  echo "unexpected response: $BODY" >&2
  exit 1
}
echo "smoke test passed"
