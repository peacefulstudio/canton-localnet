#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

EXIT_PUBLISHED=1
EXIT_BROKEN=2

usage() {
  cat <<EOF
Usage: $0 <nuget-version> <package-id>...

Fail unless none of the <package-id>s has <nuget-version> on nuget.org. A
version that is already there can never be pushed again, so a release that
reached the push would stop half-way, or report success for packages it did
not build: this check turns that into a refusal before anything is published.

Reads NUGET_FLAT_BASE (default https://api.nuget.org/v3-flatcontainer) and
runs CURL (default curl).

Exit codes: ${EXIT_PUBLISHED} a version already exists, ${EXIT_BROKEN} the lookup could not run.
EOF
}

case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac

if [ "$#" -lt 2 ]; then
  usage >&2
  exit "${EXIT_BROKEN}"
fi

version="$1"
shift
flat_base="${NUGET_FLAT_BASE:-https://api.nuget.org/v3-flatcontainer}"
curl_command="${CURL:-curl}"
body="$(mktemp)"
trap 'rm -f "${body}"' EXIT

published=()
for package_id in "$@"; do
  lowercase_id="$(tr '[:upper:]' '[:lower:]' <<<"${package_id}")"
  http_code="$(${curl_command} -sS -o "${body}" -w '%{http_code}' "${flat_base}/${lowercase_id}/index.json")" || {
    echo "::error::could not query nuget.org for ${package_id}." >&2
    exit "${EXIT_BROKEN}"
  }
  case "${http_code}" in
    404) ;;
    200)
      jq -e '.versions | type == "array"' "${body}" >/dev/null || {
        echo "::error::nuget.org returned a malformed version index for ${package_id}." >&2
        exit "${EXIT_BROKEN}"
      }
      if jq -e --arg version "${version}" '.versions | map(ascii_downcase) | index($version | ascii_downcase)' "${body}" >/dev/null; then
        published+=("${package_id}")
      fi
      ;;
    *)
      echo "::error::nuget.org answered HTTP ${http_code} for ${package_id}." >&2
      exit "${EXIT_BROKEN}"
      ;;
  esac
done

if [ "${#published[@]}" -gt 0 ]; then
  echo "::error::${version} already exists on nuget.org for: ${published[*]}. A published version is never pushed again; bump <Version>." >&2
  exit "${EXIT_PUBLISHED}"
fi
echo "${version} is not on nuget.org for: $*."
