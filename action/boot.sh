#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Orchestrates one LocalNet boot for the composite action: pre-boot
# reconciliation, up with one retry, wait-ready, then a single
# `canton-localnet env --format github` call. See
# docs/public/github-action.md for the consumer-facing contract this
# implements.
#
# Required env: CLI_PATH, ACTION_PATH, VALIDATORS, PQS, OBSERVABILITY,
# MULTI_SYNC, DIALECT, ROLES, JWT, TIMEOUT, CONFIG_INPUT, RUNNER_TEMP,
# RUNNER_ENVIRONMENT, GITHUB_WORKSPACE, GITHUB_OUTPUT, GITHUB_STEP_SUMMARY.
# Optional: ACTION_REF (shown in the job summary heading).

set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$here/lib.sh"

if [ "$DIALECT" != "native" ] && [ "$DIALECT" != "devkit" ]; then
  canton_localnet_log_error "canton-localnet action: unknown 'dialect' input '${DIALECT}' (want native or devkit)"
  exit 1
fi
if [ "$DIALECT" = "devkit" ]; then
  canton_localnet_log_error "canton-localnet action: dialect: devkit is not supported by this release of the action — only 'native' ships today. Pass dialect: native (the default), or wait for the devkit dialect PR."
  exit 1
fi
if [ -n "$ROLES" ]; then
  canton_localnet_log_error "canton-localnet action: 'roles' is a devkit-dialect input and dialect: devkit is not supported by this release — drop 'roles' (or 'dialect: native' with no roles)."
  exit 1
fi

# Only a CANTON_LOCALNET_<SLOT>_CLIENT_SECRET/_CLIENT_ID that is both set
# (non-blank after trimming, the CLI's own EnvIsSet/pick "set" definition —
# cli/internal/slot/resolve.go) and different from the fixed demo value
# this action's own booted Keycloak uses is rejected. A blank override, or
# one equal to the fixed value, is a no-op the CLI would resolve to the
# same value anyway — including the one this action's own prior `env`
# call writes back into $GITHUB_ENV, which a later 'uses:' of this action
# in the same job would otherwise see as a hostile override of itself.
overrides=()
while IFS= read -r name; do
  overrides+=("$name")
done < <(canton_localnet_client_secret_overrides "$ACTION_PATH")
while IFS= read -r name; do
  overrides+=("$name")
done < <(canton_localnet_client_id_overrides "$ACTION_PATH")
while IFS= read -r name; do
  overrides+=("$name")
done < <(canton_localnet_endpoint_overrides)
if [ "${#overrides[@]}" -gt 0 ]; then
  canton_localnet_log_error "canton-localnet action: ${overrides[*]} set in the job environment to a value that differs from this action's locally-booted stack — this action boots its own LocalNet stack with fixed demo credentials and local URLs and does not reconfigure it to match differing CANTON_LOCALNET_<SLOT>_{CLIENT_ID,CLIENT_SECRET,JSON_API_URL,TOKEN_URL} or CANTON_LOCALNET_HOST or CANTON_LOCALNET_SCAN_URL overrides; 'canton-localnet env' would export credentials or URLs that don't match the booted stack. Those overrides apply only to 'canton-localnet env' run against an externally managed LocalNet stack — unset them, or leave them at their default values, before using this action."
  exit 1
fi

# shellcheck disable=SC2207 # word-splitting a lib.sh-normalized list is intended
enabled=($(canton_localnet_normalize_validators "$VALIDATORS"))
primary_slot="${enabled[0]}"

if [ "$PQS" = "true" ]; then
  for s in "${enabled[@]}"; do
    if [ "$s" = "b" ]; then
      canton_localnet_log_warning "pqs: true with validators including 'b': compose only starts the PQS pipeline for a-validator-1 and c-validator-1 (cli/internal/compose/compose.go); CANTON_LOCALNET_B_VALIDATOR_1_PQS_* will still be exported but point at a database no pipeline ever writes to. Use 'a' and/or 'c' with pqs: true."
    fi
  done
fi

config_path=""
if [ -n "$CONFIG_INPUT" ]; then
  case "$CONFIG_INPUT" in
    /*) config_path="$CONFIG_INPUT" ;;
    *) config_path="$GITHUB_WORKSPACE/$CONFIG_INPUT" ;;
  esac
  if [ ! -f "$config_path" ]; then
    canton_localnet_log_error "canton-localnet action: 'config' points at a file that does not exist: ${config_path}"
    exit 1
  fi
  if canton_localnet_config_has_credentials "$config_path"; then
    canton_localnet_log_error "canton-localnet action: the supplied 'config' sets validators.*.auth.clientId or clientSecret — this action boots its own Keycloak with fixed LocalNet demo credentials and cannot reconfigure it at boot time; 'canton-localnet env' would export a credential the booted realm rejects. Remove auth.clientId and auth.clientSecret from the config before using this action."
    exit 1
  fi
else
  mkdir -p "$RUNNER_TEMP/canton-localnet"
  config_path="$RUNNER_TEMP/canton-localnet/canton-localnet.yaml"
  {
    echo "schemaVersion: preview-1"
    echo "modules:"
    echo "  obs: ${OBSERVABILITY}"
    echo "  pqs: ${PQS}"
    echo "multiSync: ${MULTI_SYNC}"
    echo "validators:"
    for short in a b c d; do
      enabled_flag="false"
      for want in "${enabled[@]}"; do
        [ "$want" = "$short" ] && enabled_flag="true"
      done
      canonical="$(canton_localnet_slot_canonical "$short")"
      echo "  ${canonical}:"
      echo "    enabled: ${enabled_flag}"
    done
  } >"$config_path"
fi

mkdir -p "$RUNNER_TEMP/canton-localnet"
state_file="$(canton_localnet_state_file)"
{
  printf 'CANTON_LOCALNET_ACTION_CLI_PATH=%q\n' "$CLI_PATH"
  printf 'CANTON_LOCALNET_ACTION_REPO_ROOT=%q\n' "$ACTION_PATH"
  printf 'CANTON_LOCALNET_ACTION_CONFIG_PATH=%q\n' "$config_path"
} >"$state_file"

timeout_seconds="$(canton_localnet_parse_duration "$TIMEOUT")"
deadline=$(($(date +%s) + timeout_seconds))

preboot_started="$(date +%s)"
preboot_deadline="$(canton_localnet_deadline_remaining "$deadline" "pre-boot reconciliation")"
echo "::group::Pre-boot reconciliation"
preboot_rc=0
preboot_log="$(timeout "${preboot_deadline}s" "$CLI_PATH" down --volumes --repo-root "$ACTION_PATH" --config "$config_path" 2>&1)" || preboot_rc=$?
echo "$preboot_log"
if [ "$preboot_rc" -ne 0 ]; then
  canton_localnet_log_error "canton-localnet action: pre-boot reconciliation failed (exit ${preboot_rc}) — refusing to continue; a leftover stack or volumes may contaminate the test run"
  exit 1
fi
if canton_localnet_has_leftover_containers "$preboot_log"; then
  canton_localnet_log_warning "pre-boot reconciliation found and removed leftover LocalNet containers from a previous run"
fi
echo "::endgroup::"
canton_localnet_record_phase "Pre-boot reconciliation" "$preboot_started"

echo "::group::Docker version safety check"
remaining="$(canton_localnet_deadline_remaining "$deadline" "Docker version check")"
if [ "${RUNNER_ENVIRONMENT:-github-hosted}" = "self-hosted" ]; then
  timeout "${remaining}s" bash "$ACTION_PATH/compose/scripts/check-docker-version.sh" --strict
else
  timeout "${remaining}s" bash "$ACTION_PATH/compose/scripts/check-docker-version.sh" || true
fi
echo "::endgroup::"

up_args=(up --repo-root "$ACTION_PATH" --config "$config_path")
[ "$PQS" = "true" ] && up_args+=(--pqs)
[ "$OBSERVABILITY" = "true" ] && up_args+=(--obs)
[ "$MULTI_SYNC" = "true" ] && up_args+=(--multi-sync)

down_args=(down --volumes --repo-root "$ACTION_PATH" --config "$config_path")

up_started="$(date +%s)"
echo "::group::canton-localnet up (one retry on the Splice bootstrap race)"
remaining="$(canton_localnet_deadline_remaining "$deadline" "up")"
if ! timeout "${remaining}s" "$CLI_PATH" "${up_args[@]}"; then
  canton_localnet_log_warning "canton-localnet up failed or timed out on the first attempt — capturing diagnostics, tearing down with volumes, and retrying once"
  mkdir -p "$RUNNER_TEMP/canton-localnet/diagnostics"
  timeout 30 make -C "$ACTION_PATH" status-all >"$RUNNER_TEMP/canton-localnet/diagnostics/docker-ps-attempt1.txt" 2>&1 || true
  timeout 60 make -C "$ACTION_PATH" logs-recent >"$RUNNER_TEMP/canton-localnet/diagnostics/logs-attempt1.txt" 2>&1 || true
  remaining="$(canton_localnet_deadline_remaining "$deadline" "the pre-retry teardown")"
  timeout "${remaining}s" "$CLI_PATH" "${down_args[@]}" || canton_localnet_log_warning "pre-retry teardown failed; the retry may fail with leaked state"
  sleep 30
  remaining="$(canton_localnet_deadline_remaining "$deadline" "the up retry")"
  timeout "${remaining}s" "$CLI_PATH" "${up_args[@]}"
fi
echo "::endgroup::"
canton_localnet_record_phase "Up (image pull and compose up)" "$up_started"

multi_sync_effective="$MULTI_SYNC"
if [ "$multi_sync_effective" != "true" ] && canton_localnet_multi_sync_active "$ACTION_PATH"; then
  multi_sync_effective="true"
fi

wait_ready_started="$(date +%s)"
wait_ready_slots=("${enabled[@]}" sv)
for short in "${wait_ready_slots[@]}"; do
  remaining="$(canton_localnet_deadline_remaining "$deadline" "wait-ready for ${short}")"
  prefix="$(canton_localnet_slot_prefix "$short")"
  echo "::group::Wait for ${short} (http://localhost:${prefix}975/readyz)"
  "$CLI_PATH" wait-ready --url "http://localhost:${prefix}975/readyz" --timeout "${remaining}s" --interval 5s
  echo "::endgroup::"
done

if [ "$multi_sync_effective" = "true" ]; then
  missing=()
  for want in a b d; do
    found=false
    for s in "${enabled[@]}"; do [ "$s" = "$want" ] && found=true; done
    $found || missing+=("$want")
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    canton_localnet_log_error "canton-localnet action: 'multi-sync: true' requires validators a, b and d all enabled (the app-synchronizer console script waits on all three); missing: ${missing[*]}"
    exit 1
  fi
  remaining="$(canton_localnet_deadline_remaining "$deadline" "the multi-sync wait")"
  echo "::group::Wait for the multi-synchronizer profile"
  "$CLI_PATH" wait-ready --url "http://localhost:11975/readyz" --timeout "${remaining}s" --interval 5s --synchronizers 2 --slot a-validator-1 --repo-root "$ACTION_PATH" --config "$config_path"
  echo "::endgroup::"
fi

canton_localnet_record_phase "Wait ready" "$wait_ready_started"

if [ "$PQS" = "true" ]; then
  pqs_wait_started="$(date +%s)"
  for short in "${enabled[@]}"; do
    { [ "$short" = "a" ] || [ "$short" = "c" ]; } || continue
    remaining="$(canton_localnet_deadline_remaining "$deadline" "the PQS watermark wait for ${short}")"
    prefix="$(canton_localnet_slot_prefix "$short")"
    echo "::group::Wait for the PQS watermark on ${short}"
    "$CLI_PATH" wait-ready --url "http://localhost:${prefix}975/readyz" --timeout "${remaining}s" --interval 5s \
      --pqs --slot "$(canton_localnet_slot_canonical "$short")" --repo-root "$ACTION_PATH" --config "$config_path"
    echo "::endgroup::"
  done
  canton_localnet_record_phase "Wait PQS watermark" "$pqs_wait_started"
fi

slot_flags=()
for short in "${enabled[@]}"; do
  slot_flags+=(--slot "$short")
done

report_file="$(canton_localnet_report_file)"
env_args=("$CLI_PATH" env --format github --summary-file "$report_file" --repo-root "$ACTION_PATH" --config "$config_path")
if [ "$PQS" = "true" ]; then
  env_args+=(--pqs)
fi
if [ "$JWT" = "true" ]; then
  env_args+=(--jwt)
fi
if [ "${PARTY:-false}" = "true" ]; then
  env_args+=(--party)
fi
env_args+=("${slot_flags[@]}")

primary_canonical="$(canton_localnet_slot_canonical "$primary_slot")"
printf '## canton-localnet %s\n\n' "${ACTION_REF:-(local checkout)}" >"$report_file"

env_started="$(date +%s)"
echo "::group::canton-localnet env --format github"
"${env_args[@]}"
echo "::endgroup::"
canton_localnet_record_phase "Env export" "$env_started"

{
  echo
  canton_localnet_render_timings
  echo
  canton_localnet_security_note
} >>"$report_file"
cat "$report_file" >>"$GITHUB_STEP_SUMMARY"

echo "cli-path=$CLI_PATH" >>"$GITHUB_OUTPUT"
echo "profile=$primary_canonical" >>"$GITHUB_OUTPUT"
