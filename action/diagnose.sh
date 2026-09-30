#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Collects best-effort, bounded, sanitised diagnostics into
# $RUNNER_TEMP/canton-localnet/diagnostics/ for upload by the calling
# composite step. Every command here is best-effort (`|| true`): a failing
# diagnostic must never stop teardown's `down --volumes` from running.
#
# Deliberately never collected: the rendered env, canton-localnet.yaml, or
# any compose/**/oauth2.env file. Those hold — or, for a consumer-supplied
# 'config', may hold — real secrets.
#
# Required env: RUNNER_TEMP. Reads the saved boot state
# (CANTON_LOCALNET_ACTION_CLI_PATH / _REPO_ROOT) written by action/boot.sh.

set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$here/lib.sh"

out_dir="$RUNNER_TEMP/canton-localnet/diagnostics"
mkdir -p "$out_dir"

state_file="$(canton_localnet_state_file)"
if [ -f "$state_file" ]; then
  # shellcheck disable=SC1090
  source "$state_file"
fi
repo_root="${CANTON_LOCALNET_ACTION_REPO_ROOT:-}"

if [ -n "$repo_root" ]; then
  timeout 30 make -C "$repo_root" status-all >"$out_dir/docker-ps.txt" 2>&1 || true
  timeout 60 make -C "$repo_root" status >"$out_dir/compose-status.txt" 2>&1 || true
  timeout 60 make -C "$repo_root" logs-recent >"$out_dir/compose-logs.txt" 2>&1 || true
fi
timeout 30 docker network ls >"$out_dir/docker-networks.txt" 2>&1 || true
timeout 30 docker volume ls >"$out_dir/docker-volumes.txt" 2>&1 || true

echo "Diagnostics collected in ${out_dir}:"
ls -la "$out_dir"
