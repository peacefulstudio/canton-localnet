#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Re-vendors compose modules from upstream repositories pinned in
# compose/links.csv. Used by the splice-watcher workflow on each upstream
# bump and available locally for maintainers via `make vendor`.
#
# Run from the compose/ directory:
#     cd compose && ./scripts/vendor.sh
#     cd compose && ./scripts/vendor.sh --check   # validate only, no clone
#
# CSV format (one entry per line, comments start with #):
#     REPO,COMMIT_SHA,SOURCE_PATH,DEST_PATH
#
# Every row is validated before anything is cloned or removed: COMMIT_SHA must
# be a full 40-character lowercase hex SHA, SOURCE_PATH and DEST_PATH must be
# non-empty relative paths without "." or ".." segments, and the SHA of the
# splice row must equal the one in splice.sha.

set -euo pipefail

LIST_CSV="./links.csv"
SPLICE_SHA_FILE="./splice.sha"
TMP_DIR=".tmp"

fail() {
  echo "::error::$1" >&2
  exit 1
}

is_safe_relative_path() {
  case "$1" in
    '' | /* | . | .. | ./* | ../* | */. | */.. | */./* | */../*) return 1 ;;
  esac
}

validate_row() {
  local line_number=$1 repo=$2 sha=$3 srcdir=$4 destdir=$5

  if [ -z "${repo}" ] || [ -z "${sha}" ] || [ -z "${srcdir}" ] || [ -z "${destdir}" ]; then
    fail "${LIST_CSV}:${line_number}: expected REPO,COMMIT_SHA,SOURCE_PATH,DEST_PATH with no empty field"
  fi
  if ! [[ "${sha}" =~ ^[0-9a-f]{40}$ ]]; then
    fail "${LIST_CSV}:${line_number}: COMMIT_SHA '${sha}' is not a full 40-hex commit SHA"
  fi
  if ! is_safe_relative_path "${srcdir}"; then
    fail "${LIST_CSV}:${line_number}: SOURCE_PATH '${srcdir}' must be a relative path without '.' or '..' segments"
  fi
  if ! is_safe_relative_path "${destdir}"; then
    fail "${LIST_CSV}:${line_number}: DEST_PATH '${destdir}' must be a relative path without '.' or '..' segments"
  fi
}

validate_list() {
  local line_number=0 repo sha srcdir destdir splice_sha_in_csv=""

  while IFS=',' read -r repo sha srcdir destdir || [ -n "${repo}" ]; do
    line_number=$((line_number + 1))
    case "${repo}" in
      '' | \#*) continue ;;
    esac
    validate_row "${line_number}" "${repo}" "${sha}" "${srcdir}" "${destdir}"
    case "${repo}" in
      */splice.git | */splice) splice_sha_in_csv="${sha}" ;;
    esac
  done < "${LIST_CSV}"

  if [ -f "${SPLICE_SHA_FILE}" ]; then
    local pinned_sha
    pinned_sha="$(tr -d '[:space:]' < "${SPLICE_SHA_FILE}")"
    if [ -z "${splice_sha_in_csv}" ]; then
      fail "${LIST_CSV} has no splice row to agree with ${SPLICE_SHA_FILE}"
    fi
    if [ "${pinned_sha}" != "${splice_sha_in_csv}" ]; then
      fail "${SPLICE_SHA_FILE} pins ${pinned_sha} but ${LIST_CSV} pins ${splice_sha_in_csv}"
    fi
  fi
}

if [ ! -f "${LIST_CSV}" ]; then
  echo "::error::${LIST_CSV} not found (run from the compose/ directory)" >&2
  exit 1
fi

validate_list

if [ "${1:-}" = "--check" ]; then
  echo "Vendoring pins are valid."
  exit 0
fi

trap 'rm -rf "${TMP_DIR}"' EXIT
rm -rf "${TMP_DIR}"
mkdir -p "${TMP_DIR}"

while IFS=',' read -r repo sha srcdir destdir || [ -n "${repo}" ]; do
  case "${repo}" in
    ''|\#*) continue ;;
  esac

  echo "Vendoring ${srcdir} from ${repo} @ ${sha} → ${destdir}/$(basename "${srcdir}")"

  clone_dir="${TMP_DIR}/${sha}"
  if [ ! -d "${clone_dir}" ]; then
    git clone --quiet "${repo}" "${clone_dir}"
  fi
  git -C "${clone_dir}" checkout --quiet "${sha}"

  src_full="${clone_dir}/${srcdir}"
  if [ ! -d "${src_full}" ]; then
    echo "::error::source path ${srcdir} not found at ${repo}@${sha}" >&2
    exit 1
  fi

  dest_full="${destdir}/$(basename "${srcdir}")"
  mkdir -p "${destdir}"
  rm -rf "${dest_full}"
  cp -R "${src_full}" "${dest_full}"
done < "${LIST_CSV}"

echo "Vendoring complete."
