#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

[ "$#" -ge 2 ] || { echo "usage: required-lanes-green.sh <sha> <workflow-file>..." >&2; exit 2; }

sha="$1"
shift

waiting=()
for workflow in "$@"; do
  green="$(gh run list --workflow "${workflow}" --commit "${sha}" --event push --json status,conclusion \
    --jq 'any(.[]; .status == "completed" and .conclusion == "success")' 2>&1)" \
    || { echo "gh run list for ${workflow} on ${sha} failed: ${green}"; exit 2; }
  [ "${green}" = "true" ] || waiting+=("${workflow}")
done

if [ "${#waiting[@]}" -gt 0 ]; then
  echo "${sha} is not green on: ${waiting[*]}"
  exit 1
fi
