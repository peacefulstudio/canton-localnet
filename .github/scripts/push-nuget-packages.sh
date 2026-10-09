#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 <package-dir> <nuget-version>

Push every <package-dir>/*.<nuget-version>.nupkg to nuget.org. dotnet nuget
push carries the matching .snupkg that sits beside each .nupkg. There is no
--skip-duplicate: a package that already exists fails the push, so a release
never reports success for a version it did not publish. Refuses when
<package-dir> holds no package at <nuget-version>.

Needs NUGET_API_KEY. Reads NUGET_SOURCE (default
https://api.nuget.org/v3/index.json) and runs NUGET_PUSH (default
"dotnet nuget push").
EOF
}

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

package_dir="$1"
version="$2"
nuget_source="${NUGET_SOURCE:-https://api.nuget.org/v3/index.json}"
push_command="${NUGET_PUSH:-dotnet nuget push}"

[ -n "${NUGET_API_KEY:-}" ] || {
  echo "::error::NUGET_API_KEY is unset; cannot push to nuget.org." >&2
  exit 1
}

shopt -s nullglob
packages=("${package_dir}"/*."${version}".nupkg)
[ "${#packages[@]}" -gt 0 ] || {
  echo "::error::${package_dir} holds no package at ${version}; nothing to push." >&2
  exit 1
}

for package in "${packages[@]}"; do
  echo "pushing $(basename "${package}")"
  ${push_command} "${package}" --source "${nuget_source}" --api-key "${NUGET_API_KEY}" ||
    {
      echo "::error::push failed for $(basename "${package}")." >&2
      exit 1
    }
done
