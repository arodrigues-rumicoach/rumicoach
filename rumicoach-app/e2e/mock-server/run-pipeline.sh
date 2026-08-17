#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

# Kill any stale process on port 3001
lsof -ti:3001 | xargs kill -9 2>/dev/null || true
sleep 0.5

bun install --frozen-lockfile

bun server.js > /tmp/mock-server.log 2>&1 &
SERVER_PID=$!

cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  lsof -ti:3001 | xargs kill -9 2>/dev/null || true
}
trap cleanup EXIT

echo "Waiting for server to start..."
for i in 1 2 3 4 5 6 7 8 9 10; do
  if curl -s -o /dev/null -w '%{http_code}' http://localhost:3001/v1/nonexistent 2>/dev/null | grep -q '404'; then
    break
  fi
  sleep 1
done

if ! curl -s -o /dev/null -w '%{http_code}' http://localhost:3001/v1/nonexistent | grep -q '404'; then
  echo "Server failed to start. Logs:"
  cat /tmp/mock-server.log
  exit 1
fi

echo "Server is up. Running tests..."
bash test-routes.sh
bun test-websocket.js

echo "All E2E mock server tests passed."
