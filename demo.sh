#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  R=$'\033[0m'; B=$'\033[1m'; D=$'\033[2m'
  GREEN=$'\033[38;5;42m'; TEAL=$'\033[38;5;44m'; AMBER=$'\033[38;5;214m'
  RED=$'\033[38;5;203m'; SLATE=$'\033[38;5;245m'; WHITE=$'\033[38;5;255m'
else
  R=""; B=""; D=""; GREEN=""; TEAL=""; AMBER=""; RED=""; SLATE=""; WHITE=""
fi

GATEWAY="${GATEWAY_ADDR:-http://localhost:8080}"
ADMIN="${ADMIN_ADDR:-http://localhost:8081}"

section() { echo; echo "${B}${GREEN}  ▸ $1${R}"; echo "${D}  ──────────────────────────────────────────────${R}"; }

# Guard: is the stack up?
if ! curl -fs "${ADMIN}/health" >/dev/null 2>&1; then
  echo "${RED}✘ Gateway not reachable at ${ADMIN}. Start it with ./run.sh${R}"
  exit 1
fi

echo
echo "${B}${GREEN}  RATE LIMITER — FUNCTIONAL DEMO${R}"

section "1 · System health"
curl -s "${ADMIN}/health/deep" | sed "s/^/  ${SLATE}/;s/$/${R}/"

section "2 · Rate limiting  (client-alpha = 10 requests / 10s)"
echo "  ${SLATE}Firing 14 requests — expect the first 10 to pass, then 429s.${R}"
echo
ALLOWED=0; BLOCKED=0
for i in $(seq 1 14); do
  # %{http_code} = status, %{time_total} = full round-trip seconds
  read -r CODE TIME < <(curl -s -o /dev/null \
      -w '%{http_code} %{time_total}' \
      -H 'X-Client-ID: client-alpha' \
      "${GATEWAY}/proxy/demo")
  MS=$(awk "BEGIN{printf \"%.0f\", ${TIME}*1000}")
  if [[ "$CODE" == "200" ]]; then
    ALLOWED=$((ALLOWED+1))
    printf "  ${SLATE}req %2d${R}  ${GREEN}%s ALLOW${R}   ${WHITE}%4s ms${R}\n" "$i" "$CODE" "$MS"
  elif [[ "$CODE" == "429" ]]; then
    BLOCKED=$((BLOCKED+1))
    printf "  ${SLATE}req %2d${R}  ${AMBER}%s LIMIT${R}   ${D}%4s ms${R}\n" "$i" "$CODE" "$MS"
  else
    printf "  ${SLATE}req %2d${R}  ${RED}%s ERROR${R}   ${D}%4s ms${R}\n" "$i" "$CODE" "$MS"
  fi
done
echo
printf "  ${GREEN}%d allowed${R}  ${D}·${R}  ${AMBER}%d rate-limited${R}\n" "$ALLOWED" "$BLOCKED"
if [[ "$ALLOWED" -eq 10 && "$BLOCKED" -eq 4 ]]; then
  echo "  ${GREEN}✔ Exactly 10 allowed then blocked — sliding window is accurate.${R}"
else
  echo "  ${AMBER}ℹ Counts vary if the window overlapped a previous run; re-run after ~10s for a clean 10/4.${R}"
fi

section "3 · Window reset"
echo "  ${SLATE}Waiting 10s for client-alpha's window to roll over...${R}"
sleep 10
read -r CODE TIME < <(curl -s -o /dev/null -w '%{http_code} %{time_total}' \
    -H 'X-Client-ID: client-alpha' "${GATEWAY}/proxy/demo")
MS=$(awk "BEGIN{printf \"%.0f\", ${TIME}*1000}")
if [[ "$CODE" == "200" ]]; then
  echo "  ${GREEN}✔ Request allowed again (${CODE}, ${MS} ms) — quota recovered.${R}"
else
  echo "  ${AMBER}Got ${CODE} — window may not have fully cleared yet.${R}"
fi

section "4 · Configured clients"
curl -s "${ADMIN}/api/clients" | sed "s/},{/},\n  {/g" | sed "s/^/  ${WHITE}/;s/$/${R}/"

section "5 · Analytics summary  (last 30 days, incl. latency percentiles)"
echo "  ${SLATE}Latency is measured server-side for allowed upstream calls.${R}"
echo
curl -s "${ADMIN}/api/analytics/summary?days=30" | sed "s/,/,\n  /g" | sed "s/^/  ${WHITE}/;s/$/${R}/"

echo
echo "${D}  ══════════════════════════════════════════════${R}"
echo "  ${GREEN}Demo complete.${R} ${SLATE}Open the dashboard:${R} ${WHITE}${ADMIN}${R}"
echo "  ${SLATE}Generate load and watch it live:${R} ${WHITE}./loadtest.sh${R}"
echo
