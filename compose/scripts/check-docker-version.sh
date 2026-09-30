#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

min_major_for_loopback_only_publishing=28
strict=0

for arg in "$@"; do
  case "$arg" in
    --strict) strict=1 ;;
    *) echo "usage: $0 [--strict]" >&2; exit 2 ;;
  esac
done

command -v docker > /dev/null || { echo "::error::docker is required for this check" >&2; exit 2; }

server_version=$(docker version --format '{{.Server.Version}}')
major=${server_version%%.*}

case "$major" in
  ''|*[!0-9]*)
    echo "::error::could not parse a Docker server major version from '$server_version'" >&2
    exit 2
    ;;
esac

if [ "$major" -lt "$min_major_for_loopback_only_publishing" ]; then
  message="Docker server version $server_version is below $min_major_for_loopback_only_publishing.0.0: a port published to 127.0.0.1 may still be reachable from other hosts on the same network segment on this version"
  if [ "$strict" -eq 1 ]; then
    echo "::error::$message" >&2
    exit 1
  fi
  echo "::warning::$message" >&2
  exit 0
fi

printf 'Docker server version %s is >= %d.0.0\n' "$server_version" "$min_major_for_loopback_only_publishing"
