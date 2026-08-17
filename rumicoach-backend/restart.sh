#!/bin/bash

PORT=8000
echo "Checking port $PORT..."
PIDS=$(lsof -t -i:$PORT)

if [ -n "$PIDS" ]; then
  echo "Found process(es) running on port $PORT with PID(s):"
  echo "$PIDS"
  echo "Stopping process(es)..."
  # Kill all processes listening on port 8000
  kill -9 $PIDS 2>/dev/null
  sleep 1
else
  echo "No processes found running on port $PORT."
fi

echo "Starting Rumi Backend Service..."
GO_BIN="go"
if [ -x "/opt/homebrew/bin/go" ]; then
  GO_BIN="/opt/homebrew/bin/go"
  unset GOROOT
fi
$GO_BIN run cmd/server/main.go
