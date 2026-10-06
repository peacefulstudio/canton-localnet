#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Runs `canton-localnet down --volumes` against the saved boot state
# (the CLI always passes --remove-orphans to docker compose internally;
# it is not, and must not be passed as, a CLI flag of its own — down.go
# only defines --volumes). Two modes, selected by env STRICT:
#
#   STRICT=false (the boot action's own on-failure safety net): a failed
#   down is a ::warning::, never fatal — the consumer's own .../teardown
#   call, if wired with `if: always()`, gets another chance.
#
#   STRICT=true (the teardown composite): a failed down is ::error:: and
#   sets `failed=true` in $GITHUB_OUTPUT — teardown must not read green on
#   a real failure the way the pre-Action pattern (`down || echo ::error::`)
#   did.
#
# Either way it appends a "Teardown" section to the run report
# ($RUNNER_TEMP/canton-localnet/run-report.md) and to $GITHUB_STEP_SUMMARY (when
# set) with the result and any container that was unhealthy or exited
# non-zero when teardown began. In strict mode those containers' logs are
# saved under the diagnostics directory and `unhealthy=true` is written to
# $GITHUB_OUTPUT so the teardown composite uploads them.
#
# Required env: STRICT, RUNNER_TEMP, GITHUB_OUTPUT. Optional:
# GITHUB_STEP_SUMMARY, GITHUB_JOB.
#
# `down --volumes` itself runs under a timeout: a hung docker daemon must
# not be able to run teardown's composite step past its own job timeout
# without ever reaching the diagnostics/upload/fail-the-job steps below it.

set -uo pipefail
canton_localnet_down_timeout_seconds="${CANTON_LOCALNET_DOWN_TIMEOUT_SECONDS:-120}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$here/lib.sh"

unhealthy_log_lines=200
diagnostics_dir="$RUNNER_TEMP/canton-localnet/diagnostics"

write_teardown_summary() {
  local result="$1" unhealthy_table="${2:-}" logs_pointer="" section
  if [ "$STRICT" = "true" ]; then
    logs_pointer="Logs of these containers are in the \`canton-localnet-teardown-diagnostics-${GITHUB_JOB:-job}\` artifact."
  fi
  section="$(printf '\n%s\n' "$(canton_localnet_render_teardown "$result" "$unhealthy_table" "$logs_pointer")")"
  mkdir -p "$(dirname "$(canton_localnet_report_file)")"
  echo "$section" >>"$(canton_localnet_report_file)"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    echo "$section" >>"$GITHUB_STEP_SUMMARY"
  fi
}

save_unhealthy_logs() {
  local unhealthy_table="$1" container
  mkdir -p "$diagnostics_dir"
  while IFS=$'\t' read -r container _; do
    timeout 30 docker logs --tail "$unhealthy_log_lines" "$container" >"$diagnostics_dir/unhealthy-${container}.log" 2>&1 || true
  done <<<"$unhealthy_table"
  echo "unhealthy=true" >>"$GITHUB_OUTPUT"
}

state_file="$(canton_localnet_state_file)"
if [ ! -f "$state_file" ]; then
  canton_localnet_log_error "canton-localnet action: no saved boot state at ${state_file} — the boot action (this repo's action.yml) must run earlier in this job before teardown can find what to tear down"
  write_teardown_summary "failed (no saved boot state)"
  if [ "$STRICT" = "true" ]; then
    echo "failed=true" >>"$GITHUB_OUTPUT"
    exit 1
  fi
  exit 0
fi
# shellcheck disable=SC1090
source "$state_file"

unhealthy_table="$(canton_localnet_unhealthy_containers "$CANTON_LOCALNET_ACTION_REPO_ROOT")"
if [ "$STRICT" = "true" ] && [ -n "$unhealthy_table" ]; then
  save_unhealthy_logs "$unhealthy_table"
fi

if timeout "${canton_localnet_down_timeout_seconds}s" "$CANTON_LOCALNET_ACTION_CLI_PATH" down --volumes \
  --repo-root "$CANTON_LOCALNET_ACTION_REPO_ROOT" \
  --config "$CANTON_LOCALNET_ACTION_CONFIG_PATH"; then
  write_teardown_summary "succeeded" "$unhealthy_table"
  if [ "$STRICT" = "true" ]; then
    echo "failed=false" >>"$GITHUB_OUTPUT"
  fi
  exit 0
fi

write_teardown_summary "failed" "$unhealthy_table"

if [ "$STRICT" = "true" ]; then
  canton_localnet_log_error "canton-localnet action: teardown's 'down --volumes' failed — LocalNet state may be left running on this runner"
  echo "failed=true" >>"$GITHUB_OUTPUT"
  exit 1
fi

canton_localnet_log_warning "canton-localnet action: the boot action's own teardown-on-failure 'down --volumes' failed (non-fatal here; call .../teardown with if: always() for a strict retry)"
exit 0
