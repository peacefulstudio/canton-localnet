#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Regression test for step 5 of issue #65: per-realm onboarding service-account
# clients (and the users they create) must survive a `canton-localnet down`
# (volumes preserved) + `canton-localnet up` cycle.
#
# Flow per slot:
#   1. Mint a client_credentials token against `{slot}-onboarding`.
#   2. Create a sentinel user (`restart-survival-test-<ts>`) in the slot's realm.
#   3. (Caller drives the down/up cycle between phases.)
#   4. Mint a fresh token (same client + secret) against the same realm.
#   5. GET /admin/realms/{realm}/users?username=<sentinel> — must return the user.
#
# This script supports two phases via the first positional argument:
#   create  — phase 1+2 (run after first `canton-localnet up`)
#   verify  — phase 4+5 (run after `down` + second `up`)
#
# State (sentinel username) is persisted between phases under
# tests/acceptance/.restart-survival-state.
#
# Credentials are read from the same per-slot env files the compose stack
# consumes (compose/modules/keycloak/env/{slot}-validator-1/on/oauth2.env), so
# secret rotation only needs to touch those env files. Override individual
# vars by exporting AUTH_{SLOT}_ONBOARDING_CLIENT_{ID,SECRET} before invoking
# this script.
#
# Slot coverage is configurable via SLOTS env var (default: "a b c d"). The
# issue spec ships "for slot a, repeat for b/c/d" — running all four is the
# right default; CI scenarios can narrow to SLOTS=a if runtime matters.

set -euo pipefail

PHASE="${1:-}"
if [ -z "${PHASE}" ]; then
  echo "Usage: $0 {create|verify}" >&2
  exit 2
fi

SLOTS="${SLOTS:-a b c d}"
KEYCLOAK_HOST="${KEYCLOAK_HOST:-localhost}"
KEYCLOAK_PORT="${KEYCLOAK_PORT:-8082}"
KEYCLOAK_BASE="http://${KEYCLOAK_HOST}:${KEYCLOAK_PORT}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
KEYCLOAK_ENV_DIR="${REPO_ROOT}/compose/modules/keycloak/env"
STATE_FILE="${RESTART_SURVIVAL_STATE_FILE:-${SCRIPT_DIR}/.restart-survival-state}"

require_jq() {
  if ! command -v jq > /dev/null 2>&1; then
    echo "::error::jq is required for this test but is not on PATH" >&2
    exit 2
  fi
}

slot_to_realm() {
  case "$1" in
    a) echo "AValidator1" ;;
    b) echo "BValidator1" ;;
    c) echo "CValidator1" ;;
    d) echo "DValidator1" ;;
    *) echo "::error::unknown slot: $1" >&2; return 1 ;;
  esac
}

slot_uppercase() {
  case "$1" in
    a) echo "A" ;;
    b) echo "B" ;;
    c) echo "C" ;;
    d) echo "D" ;;
    *) echo "::error::unknown slot: $1" >&2; return 1 ;;
  esac
}

# Reads the onboarding client id + secret for a slot.
# Preference order:
#   1. AUTH_{SLOT}_ONBOARDING_CLIENT_{ID,SECRET} env vars (if both set)
#   2. Parsed from compose/modules/keycloak/env/{slot}-validator-1/on/oauth2.env
#
# Emits two lines: "CLIENT_ID=...\nCLIENT_SECRET=..." on stdout.
read_onboarding_creds() {
  local slot="$1" upper id secret env_file id_var secret_var
  upper=$(slot_uppercase "${slot}")
  id_var="AUTH_${upper}_VALIDATOR_1_ONBOARDING_CLIENT_ID"
  secret_var="AUTH_${upper}_VALIDATOR_1_ONBOARDING_CLIENT_SECRET"
  id="${!id_var:-}"
  secret="${!secret_var:-}"
  if [ -n "${id}" ] && [ -n "${secret}" ]; then
    printf 'CLIENT_ID=%s\nCLIENT_SECRET=%s\n' "${id}" "${secret}"
    return 0
  fi
  env_file="${KEYCLOAK_ENV_DIR}/${slot}-validator-1/on/oauth2.env"
  if [ ! -f "${env_file}" ]; then
    echo "::error::env file ${env_file} not found and AUTH_${upper}_VALIDATOR_1_ONBOARDING_CLIENT_{ID,SECRET} not set" >&2
    return 1
  fi
  id=$(grep -E "^AUTH_${upper}_VALIDATOR_1_ONBOARDING_CLIENT_ID=" "${env_file}" | head -n1 | cut -d= -f2- | sed -E 's/[[:space:]]+#.*$//' | sed -E 's/[[:space:]]+$//')
  secret=$(grep -E "^AUTH_${upper}_VALIDATOR_1_ONBOARDING_CLIENT_SECRET=" "${env_file}" | head -n1 | cut -d= -f2- | sed -E 's/[[:space:]]+#.*$//' | sed -E 's/[[:space:]]+$//')
  if [ -z "${id}" ] || [ -z "${secret}" ]; then
    echo "::error::could not parse onboarding client id/secret from ${env_file}" >&2
    return 1
  fi
  printf 'CLIENT_ID=%s\nCLIENT_SECRET=%s\n' "${id}" "${secret}"
}

mint_token() {
  local realm="$1" client_id="$2" client_secret="$3"
  local body http_code
  body=$(mktemp)
  http_code=$(curl -s -o "${body}" -w '%{http_code}' \
    -X POST "${KEYCLOAK_BASE}/realms/${realm}/protocol/openid-connect/token" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "client_id=${client_id}" \
    --data-urlencode "client_secret=${client_secret}" \
    --data-urlencode 'grant_type=client_credentials' \
    --data-urlencode 'scope=openid')
  if [ "${http_code}" != "200" ]; then
    echo "::error::token endpoint for ${realm}/${client_id} returned HTTP ${http_code}" >&2
    cat "${body}" >&2
    rm -f "${body}"
    return 1
  fi
  jq -re .access_token < "${body}"
  local rc=$?
  rm -f "${body}"
  return $rc
}

create_user() {
  local realm="$1" token="$2" username="$3"
  local http_code
  http_code=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "${KEYCLOAK_BASE}/admin/realms/${realm}/users" \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${username}\",\"enabled\":true,\"credentials\":[{\"type\":\"password\",\"value\":\"restart-survival-pw\",\"temporary\":false}]}")
  if [ "${http_code}" != "201" ]; then
    echo "::error::POST /admin/realms/${realm}/users returned ${http_code} (expected 201)" >&2
    return 1
  fi
}

find_user() {
  local realm="$1" token="$2" username="$3"
  curl -sf -X GET "${KEYCLOAK_BASE}/admin/realms/${realm}/users?username=${username}&exact=true" \
    -H "Authorization: Bearer ${token}"
}

phase_create() {
  require_jq
  local sentinel_user="restart-survival-test-$(date +%s)-$$"
  echo "Sentinel username: ${sentinel_user}"
  echo "${sentinel_user}" > "${STATE_FILE}"

  for slot in ${SLOTS}; do
    local realm creds client_id client_secret token
    realm=$(slot_to_realm "${slot}")
    creds=$(read_onboarding_creds "${slot}")
    client_id=$(echo "${creds}" | grep '^CLIENT_ID=' | cut -d= -f2-)
    client_secret=$(echo "${creds}" | grep '^CLIENT_SECRET=' | cut -d= -f2-)

    echo "::group::[create] slot=${slot} realm=${realm} client=${client_id}"
    if ! token=$(mint_token "${realm}" "${client_id}" "${client_secret}"); then
      echo "::error::failed to mint token for ${realm}/${client_id} — does the onboarding client exist in this stack?" >&2
      echo "::endgroup::"
      return 1
    fi
    echo "  minted token (length=${#token})"

    create_user "${realm}" "${token}" "${sentinel_user}"
    echo "  created user ${sentinel_user} in ${realm} (HTTP 201)"
    echo "::endgroup::"
  done

  echo "All slots seeded. Sentinel: ${sentinel_user}"
}

phase_verify() {
  require_jq
  if [ ! -f "${STATE_FILE}" ]; then
    echo "::error::state file ${STATE_FILE} missing — did you run '$0 create' before the restart?" >&2
    return 1
  fi
  local sentinel_user
  sentinel_user=$(cat "${STATE_FILE}")
  if [ -z "${sentinel_user}" ]; then
    echo "::error::empty sentinel in ${STATE_FILE}" >&2
    return 1
  fi
  echo "Verifying sentinel: ${sentinel_user}"

  local failed=0
  for slot in ${SLOTS}; do
    local realm creds client_id client_secret token users count
    realm=$(slot_to_realm "${slot}")
    creds=$(read_onboarding_creds "${slot}")
    client_id=$(echo "${creds}" | grep '^CLIENT_ID=' | cut -d= -f2-)
    client_secret=$(echo "${creds}" | grep '^CLIENT_SECRET=' | cut -d= -f2-)

    echo "::group::[verify] slot=${slot} realm=${realm} client=${client_id}"
    if ! token=$(mint_token "${realm}" "${client_id}" "${client_secret}"); then
      echo "::error::onboarding client ${client_id} did not survive restart — token mint against ${realm} failed" >&2
      failed=1
      echo "::endgroup::"
      continue
    fi
    echo "  minted token post-restart (length=${#token})"

    if ! users=$(find_user "${realm}" "${token}" "${sentinel_user}"); then
      echo "::error::admin GET /users?username=${sentinel_user} failed against ${realm}" >&2
      failed=1
      echo "::endgroup::"
      continue
    fi
    count=$(echo "${users}" | jq 'length')
    if [ "${count}" != "1" ]; then
      echo "::error::sentinel user ${sentinel_user} not found uniquely in ${realm} (got ${count} matches)" >&2
      echo "  response: ${users}" >&2
      failed=1
      echo "::endgroup::"
      continue
    fi
    echo "  sentinel survived: 1 user matches ${sentinel_user} in ${realm}"
    echo "::endgroup::"
  done

  if [ "${failed}" -ne 0 ]; then
    echo "::error::restart-survival test FAILED — see grouped output above" >&2
    return 1
  fi
  echo "restart-survival test PASSED for slots: ${SLOTS}"
}

case "${PHASE}" in
  create) phase_create ;;
  verify) phase_verify ;;
  *) echo "Usage: $0 {create|verify}" >&2; exit 2 ;;
esac
