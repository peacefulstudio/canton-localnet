#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

expected_host_ip=${1:-127.0.0.1}

command -v jq > /dev/null || { echo "::error::jq is required for this check" >&2; exit 2; }

config_json=$(cat)

total=$(jq '[.services[].ports[]?] | length' <<<"$config_json")

if [ "$total" -eq 0 ]; then
  printf 'expected at least one published port, found 0 — check is not seeing what it expects\n' >&2
  exit 1
fi

bad=$(jq -r --arg expected "$expected_host_ip" '
  to_entries[] as $svc
  | ($svc.value.ports // [])[]
  | select((.host_ip // "") != $expected)
  | "\($svc.key) target=\(.target) published=\(.published // "<ephemeral>") host_ip=\(.host_ip // "<missing>")"
' <<<"$(jq '.services' <<<"$config_json")")

if [ -n "$bad" ]; then
  printf 'published port missing host_ip=%s:\n%s\n' "$expected_host_ip" "$bad" >&2
  exit 1
fi

printf 'checked %d published ports, all bound to host_ip=%s\n' "$total" "$expected_host_ip"
