#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  cat <<EOF2
Usage: $0 <artifact-name> <destination-dir>

Downloads the artifact <artifact-name> uploaded earlier in the current
workflow run (GITHUB_RUN_ID, GITHUB_REPOSITORY) into <destination-dir> with
\`gh run download\`, retrying a lookup that fails. The artifact API can answer
"Artifact not found" for a few seconds after an upload step reports the
artifact finalised, so the first lookup is not authoritative; a name that is
still absent after the last attempt is a real failure.

  ATTEMPTS        lookups before giving up (default 6)
  DELAY_SECONDS   pause between lookups (default 6; 5 pauses span about 30 s)

Needs GH_TOKEN with actions: read. Exits 1 when no attempt succeeds and 2 on a
wrong argument count.
EOF2
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

artifact_name="$1"
destination="$2"
attempts="${ATTEMPTS:-6}"
delay_seconds="${DELAY_SECONDS:-6}"

for attempt in $(seq 1 "$attempts"); do
  if gh run download "$GITHUB_RUN_ID" -R "$GITHUB_REPOSITORY" -n "$artifact_name" -D "$destination"; then
    exit 0
  fi
  if [ "$attempt" -lt "$attempts" ]; then
    echo "attempt $attempt/$attempts: artifact $artifact_name not downloadable yet, retrying in ${delay_seconds}s" >&2
    sleep "$delay_seconds"
  fi
done

echo "::error::artifact $artifact_name could not be downloaded from run $GITHUB_RUN_ID after $attempts attempts" >&2
exit 1
