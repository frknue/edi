#!/usr/bin/env bash
# Run the Go backend (:8080) and the Vite frontend (:5173) together.
# Ctrl-C stops both. The frontend proxies /api to the backend (see vite.config.ts).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cleanup() {
  echo ""
  echo "stopping…"
  kill 0 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo "▶ backend  → http://localhost:8080"
( cd "$ROOT/server" && EDI_DB="${EDI_DB:-edi.db}" go run . ) &

echo "▶ frontend → http://localhost:5173"
( cd "$ROOT/client" && npm run dev ) &

wait
