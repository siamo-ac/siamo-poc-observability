#!/usr/bin/env bash
# Build and run both demo services, then fire one request so the
# observability output (traces, logs, metrics) is visible.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$HOME/workspace/tools/go/bin:$PATH" GOPATH="$HOME/workspace/tools/gopath"

echo "==> building"
go build -o bin/backend ./cmd/backend
go build -o bin/frontend ./cmd/frontend

echo "==> starting backend (:8082)"
./bin/backend > backend.log 2>&1 &
BACK_PID=$!
echo "==> starting frontend (:8081)"
./bin/frontend > frontend.log 2>&1 &
FRONT_PID=$!

cleanup() { kill $BACK_PID $FRONT_PID 2>/dev/null || true; }
trap cleanup EXIT
sleep 1.5

echo
echo "==> POST /order (watch traces + JSON logs appear below)"
curl -s -X POST http://127.0.0.1:8081/order; echo
echo
echo "==> metrics (frontend)"
curl -s http://127.0.0.1:8081/metrics | grep -v '^#' | head -8
echo
echo "==> metrics (backend)"
curl -s http://127.0.0.1:8082/metrics | grep -v '^#' | head -8
echo
echo "==> what to look for:"
echo "  1. In the output above: spans named 'order', 'checkAuth' (frontend) and"
echo "     'process', 'validateOrder' (backend) — all sharing ONE TraceID."
echo "  2. backend.log / frontend.log: one JSON object per line, each with the same trace_id."
echo "  3. /metrics: http_requests_total and http_request_duration_seconds in"
echo "     Prometheus exposition format."
echo
echo "Logs saved to backend.log and frontend.log in this directory."
