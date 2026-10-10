#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat <<EOF2
Usage: $0 <version> [changelog-file]

Print the body of the "## [<version>]" section of CHANGELOG.md (default
./CHANGELOG.md), without its heading or the trailing link definitions.
Exits 1 when the section is missing or empty.
EOF2
  exit "${1:-1}"
}

[[ "${1:-}" == "-h" || "${1:-}" == "--help" ]] && usage 0
if [ $# -lt 1 ] || [ $# -gt 2 ]; then usage 1; fi

VERSION="$1"
FILE="${2:-CHANGELOG.md}"

BODY="$(awk -v heading="## [${VERSION}]" '
  /^\[[^]]+\]: / { exit }
  /^## \[/ {
    if (inside) exit
    if (index($0, heading) == 1) inside = 1
    next
  }
  inside { print }
' "$FILE" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' | sed '/./,$!d')"

if [ -z "$BODY" ]; then
  echo "::error::no non-empty '## [${VERSION}]' section in ${FILE}" >&2
  exit 1
fi
printf '%s\n' "$BODY"
