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
# Required env: STRICT, RUNNER_TEMP, GITHUB_OUTPUT.
#
# `down --volumes` itself runs under a timeout: a hung docker daemon must
# not be able to run teardown's composite step past its own job timeout
# without ever reaching the diagnostics/upload/fail-the-job steps below it.

set -uo pipefail
canton_localnet_down_timeout_seconds="${CANTON_LOCALNET_DOWN_TIMEOUT_SECONDS:-120}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$here/lib.sh"

state_file="$(canton_localnet_state_file)"
if [ ! -f "$state_file" ]; then
  canton_localnet_log_error "canton-localnet action: no saved boot state at ${state_file} — the boot action (this repo's action.yml) must run earlier in this job before teardown can find what to tear down"
  if [ "$STRICT" = "true" ]; then
    echo "failed=true" >>"$GITHUB_OUTPUT"
    exit 1
  fi
  exit 0
fi
# shellcheck disable=SC1090
source "$state_file"

if timeout "${canton_localnet_down_timeout_seconds}s" "$CANTON_LOCALNET_ACTION_CLI_PATH" down --volumes \
  --repo-root "$CANTON_LOCALNET_ACTION_REPO_ROOT" \
  --config "$CANTON_LOCALNET_ACTION_CONFIG_PATH"; then
  if [ "$STRICT" = "true" ]; then
    echo "failed=false" >>"$GITHUB_OUTPUT"
  fi
  exit 0
fi

if [ "$STRICT" = "true" ]; then
  canton_localnet_log_error "canton-localnet action: teardown's 'down --volumes' failed — LocalNet state may be left running on this runner"
  echo "failed=true" >>"$GITHUB_OUTPUT"
  exit 1
fi

canton_localnet_log_warning "canton-localnet action: the boot action's own teardown-on-failure 'down --volumes' failed (non-fatal here; call .../teardown with if: always() for a strict retry)"
exit 0
