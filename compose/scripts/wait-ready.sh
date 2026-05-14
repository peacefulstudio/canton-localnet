#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Polls the JSON Ledger API on the host-exposed a-validator-1 port until it
# answers a /readyz probe, or fails after a bounded number of attempts.
#
# Default port follows the splice convention: 11${PARTICIPANT_JSON_API_PORT_SUFFIX}
# = 11975. Override READY_URL to target a different participant or path.

set -euo pipefail

READY_URL="${READY_URL:-http://localhost:11975/readyz}"
ATTEMPTS="${ATTEMPTS:-60}"
SLEEP_SECONDS="${SLEEP_SECONDS:-5}"

echo "Polling ${READY_URL} (max ${ATTEMPTS} attempts, ${SLEEP_SECONDS}s apart)"

for i in $(seq 1 "${ATTEMPTS}"); do
  http_code="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 3 "${READY_URL}" || true)"
  if [ "${http_code}" = "200" ]; then
    echo "Ready after attempt ${i}: HTTP ${http_code}"
    exit 0
  fi
  echo "Attempt ${i}/${ATTEMPTS}: HTTP ${http_code:-no-response}"
  sleep "${SLEEP_SECONDS}"
done

echo "::error::JSON Ledger API not ready after ${ATTEMPTS} attempts at ${READY_URL}"
exit 1
