#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat <<EOF
Usage: $0

Polls the JSON Ledger API of every enabled slot until each answers a /readyz
probe, or fails after a bounded number of attempts per slot. The slots are the
ones the boot action waits on: sv, which is always probed, then a, b, c and d
unless <SLOT>_VALIDATOR_1_PROFILE is off (the toggle \`make up\` honours).

Each slot's URL follows the splice port convention, http://localhost:<prefix>975/readyz
(11975 for a-validator-1). Set READY_URL to probe that one URL instead.

Environment: READY_URL, ATTEMPTS (default 60, per slot), SLEEP_SECONDS (default 5).
EOF
}

case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac

ATTEMPTS="${ATTEMPTS:-60}"
SLEEP_SECONDS="${SLEEP_SECONDS:-5}"
SLOTS="sv a b c d"

slot_prefix() {
  case "$1" in
    sv) echo "10" ;;
    a) echo "11" ;;
    b) echo "12" ;;
    c) echo "13" ;;
    d) echo "14" ;;
  esac
}

slot_canonical() {
  case "$1" in
    sv) echo "sv-validator-1" ;;
    a) echo "a-validator-1" ;;
    b) echo "b-validator-1" ;;
    c) echo "c-validator-1" ;;
    d) echo "d-validator-1" ;;
  esac
}

slot_enabled() {
  [ "$1" = "sv" ] && return 0
  local toggle
  toggle="$(echo "$1" | tr '[:lower:]' '[:upper:]')_VALIDATOR_1_PROFILE"
  [ "${!toggle:-on}" != "off" ]
}

wait_for() {
  local label="$1" url="$2" http_code i
  echo "Polling ${label} at ${url} (max ${ATTEMPTS} attempts, ${SLEEP_SECONDS}s apart)"
  for i in $(seq 1 "${ATTEMPTS}"); do
    http_code="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 3 "${url}" || true)"
    if [ "${http_code}" = "200" ]; then
      echo "Ready after attempt ${i}: HTTP ${http_code}"
      return 0
    fi
    echo "Attempt ${i}/${ATTEMPTS}: HTTP ${http_code:-no-response}"
    sleep "${SLEEP_SECONDS}"
  done
  echo "::error::JSON Ledger API of ${label} not ready after ${ATTEMPTS} attempts at ${url}"
  return 1
}

if [ -n "${READY_URL:-}" ]; then
  wait_for "READY_URL" "${READY_URL}"
  exit 0
fi

for slot in $SLOTS; do
  slot_enabled "$slot" || continue
  wait_for "$(slot_canonical "$slot")" "http://localhost:$(slot_prefix "$slot")975/readyz" || exit 1
done
