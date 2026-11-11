#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

# Usage: required-lanes-green.sh <sha> <required-workflow-file>... [-- <other-ci-workflow-file>...]
#
# Required lanes must have run on <sha> (push event) and passed.
# Other CI lanes are path-filtered: no run on <sha> is fine, but any run that is
# still going makes the commit wait, and any run that did not succeed refuses it.
#
# Exit 0: tag-able. Exit 1: wait, the message says on what. Exit 2: refuse or
# lookup failure, the message names the failing runs.

set -euo pipefail

usage="usage: required-lanes-green.sh <sha> <workflow-file>... [-- <other-ci-workflow-file>...]"
[ "$#" -ge 2 ] || { echo "${usage}" >&2; exit 2; }

sha="$1"
shift

join_with_semicolons() {
  local joined="$1"
  shift
  for item in "$@"; do joined+="; ${item}"; done
  echo "${joined}"
}

required=()
others=()
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--" ]; then
    shift
    others=("$@")
    break
  fi
  required+=("$1")
  shift
done
[ "${#required[@]}" -ge 1 ] || { echo "${usage}" >&2; exit 2; }

waiting=()
for workflow in "${required[@]}"; do
  green="$(gh run list --workflow "${workflow}" --commit "${sha}" --event push --json status,conclusion \
    --jq 'any(.[]; .status == "completed" and .conclusion == "success")' 2>&1)" \
    || { echo "gh run list for ${workflow} on ${sha} failed: ${green}"; exit 2; }
  [ "${green}" = "true" ] || waiting+=("${workflow}")
done

refused=()
running=()
for workflow in "${others[@]+"${others[@]}"}"; do
  runs="$(gh run list --workflow "${workflow}" --commit "${sha}" --event push --json databaseId,status,conclusion \
    --jq '.[] | "\(.databaseId) \(.status) \(.conclusion // "")"' 2>&1)" \
    || { echo "gh run list for ${workflow} on ${sha} failed: ${runs}"; exit 2; }
  while read -r run_id run_status run_conclusion; do
    [ -n "${run_id}" ] || continue
    if [ "${run_status}" != "completed" ]; then
      running+=("${workflow} run ${run_id} is ${run_status}")
    else
      case "${run_conclusion}" in
        success | skipped | neutral) ;;
        *) refused+=("${workflow} run ${run_id} concluded ${run_conclusion}") ;;
      esac
    fi
  done <<< "${runs}"
done

if [ "${#refused[@]}" -gt 0 ]; then
  echo "${sha} is red, refusing to tag: $(join_with_semicolons "${refused[@]}")"
  exit 2
fi

if [ "${#waiting[@]}" -gt 0 ] || [ "${#running[@]}" -gt 0 ]; then
  message="${sha} is not green"
  [ "${#waiting[@]}" -eq 0 ] || message+=" on: ${waiting[*]}"
  [ "${#running[@]}" -eq 0 ] || message+=" (still running: $(join_with_semicolons "${running[@]}"))"
  echo "${message}"
  exit 1
fi
