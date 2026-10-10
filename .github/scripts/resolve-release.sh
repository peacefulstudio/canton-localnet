#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 <tag-or-version> [repo-root]

Run at the release tag's checkout (repo-root defaults to .). Refuses unless
the argument names a release the tree can ship:

  - it is v<X.Y.Z>-<N> or v<X.Y.Z>-<N>.<label>, with or without the leading v;
  - it equals <Version> in csharp/Directory.Build.props;
  - CHANGELOG.md holds a non-empty "## [<version>]" section.

Prints key=value lines (appended to \$GITHUB_OUTPUT when it is set):
  tag          v<version>
  version      <version>
  nuget        the NuGet spelling: X.Y.Z.N, or X.Y.Z.N-<label> for a preview
  prerelease   false for X.Y.Z-N, true for a labelled preview

Exit codes: 0 resolved, 1 refused.
EOF
}

case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  usage >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
version="${1#v}"
root="${2:-.}"

refuse() {
  echo "::error::$*" >&2
  exit 1
}

if [[ "${version}" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-([1-9][0-9]*)$ ]]; then
  nuget="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"
  prerelease=false
elif [[ "${version}" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-([1-9][0-9]*)\.([0-9A-Za-z][0-9A-Za-z.]*)$ ]]; then
  nuget="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}-${BASH_REMATCH[3]}"
  prerelease=true
else
  refuse "'${1}' matches neither v<X.Y.Z>-<N> nor v<X.Y.Z>-<N>.<label>. See RELEASE.md, Version-format rule."
fi

props="${root}/csharp/Directory.Build.props"
[ -r "${props}" ] || refuse "${props} is missing or unreadable."
source_version="$(sed -n 's:.*<Version>\(.*\)</Version>.*:\1:p' "${props}" | head -n 1 | tr -d '[:space:]')"
[ "${source_version}" = "${version}" ] ||
  refuse "Release v${version} does not match <Version> '${source_version}' in ${props} at the tagged commit."

"${script_dir}/changelog-section.sh" "${version}" "${root}/CHANGELOG.md" >/dev/null ||
  refuse "CHANGELOG.md has no non-empty section for ${version}."

{
  echo "tag=v${version}"
  echo "version=${version}"
  echo "nuget=${nuget}"
  echo "prerelease=${prerelease}"
} | tee -a "${GITHUB_OUTPUT:-/dev/null}"
