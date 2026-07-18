#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# ---- minimalist, New Relic-style palette ----
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  R=$'\033[0m'; B=$'\033[1m'; D=$'\033[2m'
  GREEN=$'\033[38;5;42m'; TEAL=$'\033[38;5;44m'; AMBER=$'\033[38;5;214m'
  RED=$'\033[38;5;203m'; SLATE=$'\033[38;5;245m'; WHITE=$'\033[38;5;255m'
else
  R=""; B=""; D=""; GREEN=""; TEAL=""; AMBER=""; RED=""; SLATE=""; WHITE=""
fi

ADMIN_URL="http://localhost:8081"
GATEWAY_URL="http://localhost:8080"
MOCK_URL="http://localhost:9090"

# Resolve the docker compose command (v2 plugin or legacy binary).
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  echo "${RED}✘ Docker Compose not found. Install Docker Desktop and retry.${R}"
  exit 1
fi

banner() {
  echo
  echo "${B}${GREEN}  ██ RATE LIMITER — Global API Gateway${R}"
  echo "${D}  ──────────────────────────────────────────────${R}"
}

case "${1:-up}" in
  down)
    banner
    echo "  ${SLATE}Stopping stack...${R}"
    $DC down
    echo "  ${GREEN}✔ Stopped.${R}"
    exit 0
    ;;
  logs)
    exec $DC logs -f
    ;;
  restart)
    $DC down
    ;;
esac

banner
echo "  ${SLATE}Building images and starting services...${R}"
echo "  ${D}(first run pulls base images + compiles Go/React — give it a minute)${R}"
echo

$DC up --build -d

# ---- wait for the gateway to report healthy ----
echo
printf "  ${SLATE}Waiting for the gateway to become healthy${R}"
HEALTHY=0
for _ in $(seq 1 60); do
  if curl -fs "${ADMIN_URL}/health" >/dev/null 2>&1; then
    HEALTHY=1
    break
  fi
  printf "."
  sleep 2
done
echo

if [[ "$HEALTHY" -ne 1 ]]; then
  echo "  ${AMBER}⚠ Gateway did not report healthy in time.${R}"
  echo "  ${SLATE}Check logs with:${R} ${WHITE}./run.sh logs${R}"
  exit 1
fi

# ---- pretty summary of the live endpoints ----
echo
echo "  ${B}${GREEN}✔ Stack is up and healthy${R}"
echo "${D}  ══════════════════════════════════════════════${R}"
printf "  ${B}${TEAL}▸ DASHBOARD${R}   ${WHITE}%s${R}   ${D}← open this${R}\n" "$ADMIN_URL"
printf "  ${SLATE}  Gateway${R}      ${WHITE}%s${R}   ${D}(send proxied traffic here)${R}\n" "$GATEWAY_URL"
printf "  ${SLATE}  Admin API${R}    ${WHITE}%s/api${R}\n" "$ADMIN_URL"
printf "  ${SLATE}  Mock API${R}     ${WHITE}%s${R}   ${D}(simulated third-party service)${R}\n" "$MOCK_URL"
echo "${D}  ──────────────────────────────────────────────${R}"
echo "  ${SLATE}Next steps:${R}"
printf "    ${WHITE}./demo.sh${R}       ${D}functional test — see rate limiting + latencies${R}\n"
printf "    ${WHITE}./loadtest.sh${R}   ${D}fire >10k requests and watch the dashboard${R}\n"
printf "    ${WHITE}./run.sh logs${R}   ${D}tail logs   ·   ${WHITE}./run.sh down${R} ${D}stop${R}\n"
echo

# ---- best-effort: open the dashboard in the browser ----
if command -v open >/dev/null 2>&1; then
  open "$ADMIN_URL" >/dev/null 2>&1 || true
elif command -v xdg-open >/dev/null 2>&1; then
  xdg-open "$ADMIN_URL" >/dev/null 2>&1 || true
fi
