#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat <<EOF2
Usage: $0 [--shell-default] <env-file> <KEY>

Prints the value of KEY from a compose env file, parsed the way the CLI's
ReadEnvFile parses it (cli/internal/slot/resolve.go): blank and # lines are
skipped, whitespace around key and value is trimmed, a trailing comment that
starts with whitespace then # is dropped, one pair of matching quotes is
removed, and the last definition wins. Uses only bash and POSIX awk, so it
behaves the same on GNU and BSD userlands.

  --shell-default  KEY must be defined as \${KEY:-default}; print the default.

Exits 1 when KEY is absent or --shell-default finds another shape, and 2 on a
missing file or a wrong argument count.
EOF2
}

shell_default=0
if [ "${1:-}" = "--shell-default" ]; then
  shell_default=1
  shift
fi
case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac
if [ "$#" -ne 2 ]; then
  usage >&2
  exit 2
fi

env_file="$1"
key="$2"
if [ ! -f "$env_file" ]; then
  echo "read-env-value: $env_file: no such file" >&2
  exit 2
fi

value="$(awk -v key="$key" '
  function trim(s) {
    sub(/^[ \t\r]+/, "", s)
    sub(/[ \t\r]+$/, "", s)
    return s
  }
  function strip_inline_comment(s,    i) {
    for (i = 1; i <= length(s); i++) {
      if (substr(s, i, 1) == "#" && (i == 1 || substr(s, i - 1, 1) ~ /[ \t]/)) {
        return substr(s, 1, i - 1)
      }
    }
    return s
  }
  function strip_paired_quotes(s,    first) {
    first = substr(s, 1, 1)
    if (length(s) >= 2 && (first == "\"" || first == "\047") && substr(s, length(s), 1) == first) {
      return substr(s, 2, length(s) - 2)
    }
    return s
  }
  {
    line = trim($0)
    if (line == "" || substr(line, 1, 1) == "#") next
    eq = index(line, "=")
    if (eq <= 1 || trim(substr(line, 1, eq - 1)) != key) next
    found = 1
    value = strip_paired_quotes(trim(strip_inline_comment(substr(line, eq + 1))))
  }
  END {
    if (!found) exit 1
    print value
  }
' "$env_file")" || {
  echo "read-env-value: $env_file: $key is not defined" >&2
  exit 1
}

if [ "$shell_default" -eq 1 ]; then
  prefix="\${$key:-"
  if [ "${value#"$prefix"}" = "$value" ] || [ "${value%\}}" = "$value" ]; then
    echo "read-env-value: $env_file: $key is not defined as \${$key:-default}" >&2
    exit 1
  fi
  value="${value#"$prefix"}"
  value="${value%\}}"
fi

printf '%s\n' "$value"
