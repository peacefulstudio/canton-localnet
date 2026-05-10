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
#
# CSV format (one entry per line, comments start with #):
#     REPO,COMMIT_SHA,SOURCE_PATH,DEST_PATH

set -euo pipefail

LIST_CSV="./links.csv"
TMP_DIR=".tmp"

if [ ! -f "${LIST_CSV}" ]; then
  echo "::error::${LIST_CSV} not found (run from the compose/ directory)" >&2
  exit 1
fi

trap 'rm -rf "${TMP_DIR}"' EXIT
rm -rf "${TMP_DIR}"
mkdir -p "${TMP_DIR}"

while IFS=',' read -r repo sha srcdir destdir; do
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
