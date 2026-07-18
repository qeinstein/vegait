#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  R=$'\033[0m'; GREEN=$'\033[38;5;42m'; SLATE=$'\033[38;5;245m'; RED=$'\033[38;5;203m'; WHITE=$'\033[38;5;255m'
else
  R=""; GREEN=""; SLATE=""; RED=""; WHITE=""
fi

ADMIN="${ADMIN_ADDR:-http://localhost:8081}"

# Default to a healthy volume above the 10k requirement unless the caller
# already passed their own -n flag.
ARGS=("$@")
if [[ ! " ${ARGS[*]} " == *" -n "* ]]; then
  ARGS=(-n 12000 "${ARGS[@]}")
fi

if ! curl -fs "${ADMIN}/health" >/dev/null 2>&1; then
  echo "${RED}✘ Gateway not reachable at ${ADMIN}. Start it with ./run.sh${R}"
  exit 1
fi

echo "${SLATE}Tip: open ${WHITE}${ADMIN}${SLATE} and switch to the Live view to watch this in real time.${R}"

if command -v go >/dev/null 2>&1; then
  echo "${SLATE}Running load generator locally (go run)...${R}"
  ( cd backend && GOWORK=off go run ./cmd/loadgen "${ARGS[@]}" )
else
  # Fall back to the loadgen binary shipped inside the gateway container.
  if docker compose version >/dev/null 2>&1; then DC="docker compose"; else DC="docker-compose"; fi
  echo "${SLATE}Go not found on host — running loadgen inside the container...${R}"
  $DC exec -T rate-limiter /app/loadgen "${ARGS[@]}"
fi

echo
echo "${GREEN}✔ Load test finished.${R} ${SLATE}Refresh the dashboard to see the aggregated analytics.${R}"
