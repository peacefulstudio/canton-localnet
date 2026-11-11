#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

EXIT_REFUSED=1
EXIT_BROKEN=2
BLKID_NOTHING_FOUND=2
PROBE_BYTES=1048576

usage() {
  cat <<EOF
Usage: $0 <device>

Format <device> with ext4 only when it is provably blank: blkid -p reports no
TYPE and the first MiB is all zero. A device blkid reports a TYPE for is left
alone.

Anything else refuses to format and exits ${EXIT_REFUSED}: blkid failing for any
reason other than finding nothing (a missing binary, a probe error, an
ambiguous signature), a partition table, or data in the first MiB. A reattached
ledger volume must never be reformatted on a doubtful probe.

Exit codes: 0 the device is formatted or already has a filesystem,
${EXIT_REFUSED} refused to format, ${EXIT_BROKEN} the script could not run.
EOF
}

case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac

if [ "$#" -ne 1 ]; then
  usage >&2
  exit "${EXIT_BROKEN}"
fi

device="$1"

refuse() {
  echo "format-blank-volume: FATAL: $*" >&2
  exit "${EXIT_REFUSED}"
}

blkid_status=0
blkid_output="$(blkid -p "$device" 2>&1)" || blkid_status=$?

if [ "$blkid_status" -ne 0 ] && [ "$blkid_status" -ne "$BLKID_NOTHING_FOUND" ]; then
  refuse "blkid -p $device failed with exit $blkid_status (${blkid_output}) — refusing to format"
fi

if grep -qE '(^|[[:space:]])TYPE=' <<<"$blkid_output"; then
  echo "format-blank-volume: $device already has a filesystem: $blkid_output"
  exit 0
fi

non_zero_bytes="$(head -c "$PROBE_BYTES" "$device" | tr -d '\0' | wc -c)"
bytes_read="$(head -c "$PROBE_BYTES" "$device" | wc -c)"

if [ "$bytes_read" -ne "$PROBE_BYTES" ]; then
  refuse "read ${bytes_read} of ${PROBE_BYTES} bytes from $device — refusing to format"
fi

if [ "$non_zero_bytes" -ne 0 ]; then
  refuse "$device reports no filesystem TYPE but its first MiB holds data (${non_zero_bytes} non-zero bytes${blkid_output:+, blkid: ${blkid_output}}) — refusing to format"
fi

echo "format-blank-volume: $device is blank — formatting ext4"
mkfs.ext4 -F "$device"
