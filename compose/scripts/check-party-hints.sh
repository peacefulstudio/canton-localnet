#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

status=0
distinct_slots=0
seen_keys=''

while IFS= read -r line; do
  case "$line" in
    *_VALIDATOR_1_PARTY_HINT:*) ;;
    *) continue ;;
  esac

  key=${line%%:*}
  key=${key##* }
  value=${line#*: }
  value=${value//\"/}
  value=${value//\'/}
  # Assumes key prefixes contain only [A-Z_]; a digit-leading prefix would silently produce a wrong slot string.
  slot=$(printf '%s' "${key%_PARTY_HINT}" | tr '[:upper:]_' '[:lower:]-')

  case " $seen_keys " in
    *" $key "*) ;;
    *)
      seen_keys="$seen_keys $key"
      distinct_slots=$((distinct_slots + 1))
      ;;
  esac

  if [ "$value" != "$slot" ]; then
    printf 'party hint mismatch: %s=%s, expected %s\n' "$key" "$value" "$slot" >&2
    status=1
  fi
done

if [ "$distinct_slots" -ne 4 ]; then
  printf 'expected 4 distinct slot party hints, found %d (%s ) — check is not seeing what it expects\n' "$distinct_slots" "$seen_keys" >&2
  exit 1
fi

exit "$status"
