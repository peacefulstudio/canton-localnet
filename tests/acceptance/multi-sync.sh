#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Regression test for the multi-synchronizer bootstrap
# (compose/modules/localnet/conf/console/app-synchronizer.sc).
#
# That script is vendored from upstream splice and rewritten on every version
# bump, yet nothing else in this repo executes it: it runs only when
# MULTI_SYNC=true (docker/console/entrypoint.sh), which is gated behind the
# `multi-sync` compose profile. Before this test the whole path was covered
# only by a weekly, non-gating lane in a consumer repo.
#
# Assertions, run after `canton-localnet up --multi-sync`:
#
#   1. multi-sync-startup exited 0. The bootstrap ends with a retry_until_true
#      that re-reads the trust certificates and confirms EnableMultiSynchronizer
#      became effective, so a zero exit is the flag actually landing — not just
#      the propose call returning. `up -d` already blocks on this via
#      multi-sync-ready's service_completed_successfully, but assert it
#      explicitly so the signal survives a change to that gating.
#   2. Its logs carry no Scala exception. retry_until_true throws on expiry,
#      and a throw is the failure mode this test exists to catch.
#   3. a/b/d each report 2 connected synchronizers (global + app-synchronizer)
#      over the JSON Ledger API — independent confirmation that the topology
#      the flag describes is real, not just that the console said so.
#   4. Negative control: c-validator-1 is healthy but reports only 1. c is
#      deliberately absent from the app-synchronizer wiring, so a bootstrap
#      that over-applied the profile would show up here and nowhere else.
#
# Slot coverage is configurable via MULTI_SYNC_SLOTS (default "a-validator-1
# b-validator-1 d-validator-1") to match the `Seq` in app-synchronizer.sc; if
# that Seq ever changes, change this to match and the two stay in lockstep.

set -euo pipefail

CLI="${CANTON_LOCALNET_BIN:-canton-localnet}"
SLOTS="${MULTI_SYNC_SLOTS:-a-validator-1 b-validator-1 d-validator-1}"
CONTROL_SLOT="${MULTI_SYNC_CONTROL_SLOT:-c-validator-1}"
BOOTSTRAP_CONTAINER="${MULTI_SYNC_CONTAINER:-multi-sync-startup}"
TIMEOUT="${MULTI_SYNC_TIMEOUT:-300s}"
CONTROL_TIMEOUT="${MULTI_SYNC_CONTROL_TIMEOUT:-45s}"
POLL_INTERVAL="${MULTI_SYNC_POLL_INTERVAL:-5s}"

fail() {
  echo "::error::$*" >&2
  exit 1
}

# `wait-ready --slot` only steers the connected-synchronizers query; the readyz
# poll it runs first always uses --url, which defaults to a-validator-1's
# 11975. Passing only --slot would poll the wrong participant and make every
# check below meaningless, so map slot -> host port and always pass both.
slot_url() {
  case "$1" in
    sv-validator-1) echo "http://localhost:10975/readyz" ;;
    a-validator-1)  echo "http://localhost:11975/readyz" ;;
    b-validator-1)  echo "http://localhost:12975/readyz" ;;
    c-validator-1)  echo "http://localhost:13975/readyz" ;;
    d-validator-1)  echo "http://localhost:14975/readyz" ;;
    *) echo "::error::unknown slot '$1' — add its port to slot_url()" >&2; exit 2 ;;
  esac
}

echo "== 1. ${BOOTSTRAP_CONTAINER} completed successfully =="
if ! docker inspect "${BOOTSTRAP_CONTAINER}" > /dev/null 2>&1; then
  fail "container '${BOOTSTRAP_CONTAINER}' does not exist — the multi-sync profile did not start. Was 'up' run without --multi-sync?"
fi

state="$(docker inspect -f '{{.State.Status}}' "${BOOTSTRAP_CONTAINER}")"
exit_code="$(docker inspect -f '{{.State.ExitCode}}' "${BOOTSTRAP_CONTAINER}")"
echo "state=${state} exit_code=${exit_code}"
if [ "${state}" != "exited" ]; then
  fail "${BOOTSTRAP_CONTAINER} is '${state}', expected 'exited' — the bootstrap never finished"
fi
if [ "${exit_code}" != "0" ]; then
  fail "${BOOTSTRAP_CONTAINER} exited ${exit_code} — the multi-synchronizer bootstrap failed. Its logs follow in the diagnostics artifact."
fi

echo "== 2. no exception in ${BOOTSTRAP_CONTAINER} logs =="
logs="$(docker logs "${BOOTSTRAP_CONTAINER}" 2>&1 || true)"
# Deliberately narrow. A generic stack-trace or IllegalStateException pattern
# also matches benign retry chatter in Canton's console logs, and a test that
# fails on a healthy stack is worse than one that misses an edge case. These
# two strings do not appear on a successful bootstrap: retry_until_true prints
# the first on expiry, the console the second on a rejected admin command.
if echo "${logs}" | grep -qE 'Condition never became true|CommandExecutionFailed'; then
  echo "${logs}" | tail -40 >&2
  fail "${BOOTSTRAP_CONTAINER} logs contain a Scala failure despite exit 0"
fi
echo "clean"

echo "== 3. ${SLOTS} each connected to 2 synchronizers =="
for slot in ${SLOTS}; do
  echo "::group::${slot}"
  "${CLI}" wait-ready \
    --url "$(slot_url "${slot}")" \
    --slot "${slot}" \
    --synchronizers 2 \
    --timeout "${TIMEOUT}" \
    --interval "${POLL_INTERVAL}" \
    || fail "${slot} never reported 2 connected synchronizers — the app-synchronizer connection or its feature flag did not take"
  echo "::endgroup::"
done

echo "== 4. negative control: ${CONTROL_SLOT} healthy but on 1 synchronizer =="
control_url="$(slot_url "${CONTROL_SLOT}")"
"${CLI}" wait-ready \
  --url "${control_url}" \
  --timeout "${CONTROL_TIMEOUT}" \
  --interval "${POLL_INTERVAL}" \
  || fail "${CONTROL_SLOT} is not healthy — the negative control below would pass for the wrong reason"

# Capture rather than discard. On the happy path this swallows ~9 lines of
# "waiting for synchronizers: 1/2" chatter; on the failure path it is the only
# evidence of what the participant actually reported, which is precisely what
# someone opening this log needs.
if control_out="$("${CLI}" wait-ready \
     --url "${control_url}" \
     --slot "${CONTROL_SLOT}" \
     --synchronizers 2 \
     --timeout "${CONTROL_TIMEOUT}" \
     --interval "${POLL_INTERVAL}" 2>&1)"; then
  echo "${control_out}" >&2
  fail "${CONTROL_SLOT} reports 2 connected synchronizers but is deliberately not wired to app-synchronizer — the multi-sync profile is over-applying"
fi
echo "confirmed on 1 synchronizer"

echo "multi-sync bootstrap verified"
