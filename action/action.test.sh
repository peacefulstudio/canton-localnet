#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Unit-level tests for action/*.sh's own logic — validator/timeout
# parsing, and the resolve-cli/down/diagnose failure branches — run with a
# stub `canton-localnet` on PATH or no daemon at all, never a real LocalNet
# boot. This is action-selftest.yaml's T1 job: it proves the shell logic
# without paying for a multi-minute compose boot, following the same
# plain-bash-test convention as scripts/gitpublic-paths.test.sh (this repo
# has no bats dependency).
#
# Usage: action/action.test.sh

set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$here/lib.sh"

failures=0
tests_run=0

assert_eq() {
  local desc="$1" want="$2" got="$3"
  tests_run=$((tests_run + 1))
  if [ "$want" != "$got" ]; then
    echo "FAIL: $desc: want [$want], got [$got]"
    failures=$((failures + 1))
  else
    echo "ok: $desc"
  fi
}

assert_status() {
  local desc="$1" want_status="$2" got_status="$3"
  tests_run=$((tests_run + 1))
  if [ "$want_status" != "$got_status" ]; then
    echo "FAIL: $desc: want exit $want_status, got $got_status"
    failures=$((failures + 1))
  else
    echo "ok: $desc"
  fi
}

assert_contains() {
  local desc="$1" haystack="$2" needle="$3"
  tests_run=$((tests_run + 1))
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "FAIL: $desc: expected to find [$needle]"
    failures=$((failures + 1))
  else
    echo "ok: $desc"
  fi
}

## canton_localnet_normalize_validators

got="$(canton_localnet_normalize_validators 'a,b, b ,a-validator-1')"
assert_eq "normalize dedups and accepts canonical form" "a b" "$got"

got="$(canton_localnet_normalize_validators 'a b c d')"
assert_eq "normalize accepts the full set" "a b c d" "$got"

if canton_localnet_normalize_validators 'sv' >/dev/null 2>&1; then
  echo "FAIL: normalize should reject sv"
  failures=$((failures + 1))
else
  echo "ok: normalize rejects sv"
fi
tests_run=$((tests_run + 1))

if canton_localnet_normalize_validators 'e' >/dev/null 2>&1; then
  echo "FAIL: normalize should reject an unknown slot"
  failures=$((failures + 1))
else
  echo "ok: normalize rejects an unknown slot"
fi
tests_run=$((tests_run + 1))

if canton_localnet_normalize_validators '' >/dev/null 2>&1; then
  echo "FAIL: normalize should reject an empty list"
  failures=$((failures + 1))
else
  echo "ok: normalize rejects an empty list"
fi
tests_run=$((tests_run + 1))

## canton_localnet_has_leftover_containers

if canton_localnet_has_leftover_containers " Container canton-localnet-postgres-1  Removed"; then
  echo "ok: has_leftover_containers matches a literal compose down status line ending in the verb"
else
  echo "FAIL: has_leftover_containers should match a literal compose down status line ending in the verb"
  failures=$((failures + 1))
fi
tests_run=$((tests_run + 1))

if canton_localnet_has_leftover_containers " Network localnet  Removing"; then
  echo "ok: has_leftover_containers matches an in-progress verb ending the line"
else
  echo "FAIL: has_leftover_containers should match an in-progress verb ending the line"
  failures=$((failures + 1))
fi
tests_run=$((tests_run + 1))

if canton_localnet_has_leftover_containers "no leftovers here"; then
  echo "FAIL: has_leftover_containers should not match a line with no compose status verb"
  failures=$((failures + 1))
else
  echo "ok: has_leftover_containers does not match a line with no compose status verb"
fi
tests_run=$((tests_run + 1))

## canton_localnet_parse_duration

assert_eq "parse_duration 15m" "900" "$(canton_localnet_parse_duration 15m)"
assert_eq "parse_duration 90s" "90" "$(canton_localnet_parse_duration 90s)"
assert_eq "parse_duration 1h" "3600" "$(canton_localnet_parse_duration 1h)"

if canton_localnet_parse_duration '15' >/dev/null 2>&1; then
  echo "FAIL: parse_duration should reject a bare number"
  failures=$((failures + 1))
else
  echo "ok: parse_duration rejects a bare number"
fi
tests_run=$((tests_run + 1))


## canton_localnet_up_attempt_seconds

assert_eq "up_attempt_seconds: no cap uses the whole remaining budget" "600" "$(canton_localnet_up_attempt_seconds 600 '')"
assert_eq "up_attempt_seconds: a cap below the remaining budget wins" "120" "$(canton_localnet_up_attempt_seconds 600 2m)"
assert_eq "up_attempt_seconds: a cap above the remaining budget is narrowed to it" "600" "$(canton_localnet_up_attempt_seconds 600 1h)"
assert_eq "up_attempt_seconds: a zero cap uses the whole remaining budget, never an unbounded attempt" "600" "$(canton_localnet_up_attempt_seconds 600 0s)"
if canton_localnet_up_attempt_seconds 600 bogus >/dev/null 2>&1; then
  echo "FAIL: up_attempt_seconds should reject a malformed cap"
  failures=$((failures + 1))
else
  echo "ok: up_attempt_seconds rejects a malformed cap"
fi
tests_run=$((tests_run + 1))

## canton_localnet_deadline_remaining

past="$(($(date +%s) - 10))"
if canton_localnet_deadline_remaining "$past" "a test phase" >/dev/null 2>&1; then
  echo "FAIL: deadline_remaining should fail once the deadline has passed"
  failures=$((failures + 1))
else
  echo "ok: deadline_remaining fails once the deadline has passed"
fi
tests_run=$((tests_run + 1))

future="$(($(date +%s) + 60))"
got="$(canton_localnet_deadline_remaining "$future" "a test phase")"
tests_run=$((tests_run + 1))
if [ "$got" -lt 1 ] || [ "$got" -gt 60 ]; then
  echo "FAIL: deadline_remaining: want 1..60, got $got"
  failures=$((failures + 1))
else
  echo "ok: deadline_remaining returns the remaining budget"
fi

## resolve-cli.sh fails closed on cli: release with a non-tag ref, before any network call

RUNNER_TEMP="$(mktemp -d)"
export RUNNER_TEMP
out="$(mktemp)"
path_out="$(mktemp)"
set +e
CLI_MODE=release ACTION_PATH="$here/.." ACTION_REF="" GITHUB_OUTPUT="$out" GITHUB_PATH="$path_out" \
  bash "$here/resolve-cli.sh" >/tmp/resolve-cli-release.log 2>&1
status=$?
set -e
assert_status "resolve-cli: cli: release with a non-tag ref fails closed" "1" "$status"
assert_contains "resolve-cli: cli: release error names the fix" "$(cat /tmp/resolve-cli-release.log)" "cli: source"

## down.sh (STRICT=true) with no saved boot state fails closed

RUNNER_TEMP="$(mktemp -d)"
export RUNNER_TEMP
out="$(mktemp)"
set +e
STRICT=true GITHUB_OUTPUT="$out" bash "$here/down.sh" >/tmp/down-no-state.log 2>&1
status=$?
set -e
assert_status "down.sh (strict): no saved state fails closed" "1" "$status"
assert_contains "down.sh (strict): no saved state sets failed=true" "$(cat "$out")" "failed=true"

## down.sh (STRICT=true) with a stub canton-localnet whose `down` fails

RUNNER_TEMP="$(mktemp -d)"
export RUNNER_TEMP
mkdir -p "$RUNNER_TEMP/canton-localnet"
stub="$(mktemp -d)/canton-localnet"
cat >"$stub" <<'EOF'
#!/usr/bin/env bash
if [ "$1" = "down" ]; then
  echo "stub: down always fails" >&2
  exit 7
fi
exit 0
EOF
chmod +x "$stub"
{
  echo "CANTON_LOCALNET_ACTION_CLI_PATH=$stub"
  echo "CANTON_LOCALNET_ACTION_REPO_ROOT=$here/.."
  echo "CANTON_LOCALNET_ACTION_CONFIG_PATH=/dev/null"
} >"$(canton_localnet_state_file)"
out="$(mktemp)"
set +e
STRICT=true GITHUB_OUTPUT="$out" bash "$here/down.sh" >/tmp/down-strict-fail.log 2>&1
status=$?
set -e
assert_status "down.sh (strict): a failing stub 'down' fails closed" "1" "$status"
assert_contains "down.sh (strict): a failing stub 'down' sets failed=true" "$(cat "$out")" "failed=true"
assert_contains "down.sh (strict): a failing stub 'down' logs ::error::" "$(cat /tmp/down-strict-fail.log)" "::error::"

## down.sh (STRICT=false) with the same failing stub is non-fatal

out="$(mktemp)"
set +e
STRICT=false GITHUB_OUTPUT="$out" bash "$here/down.sh" >/tmp/down-nonstrict-fail.log 2>&1
status=$?
set -e
assert_status "down.sh (non-strict): a failing stub 'down' does not fail the step" "0" "$status"
assert_contains "down.sh (non-strict): a failing stub 'down' logs ::warning::" "$(cat /tmp/down-nonstrict-fail.log)" "::warning::"

## diagnose.sh never touches canton-localnet.yaml or an oauth2.env, and never fails the job

RUNNER_TEMP="$(mktemp -d)"
export RUNNER_TEMP
set +e
bash "$here/diagnose.sh" >/tmp/diagnose.log 2>&1
status=$?
set -e
assert_status "diagnose.sh: runs to completion with no saved state" "0" "$status"
if grep -qi "oauth2.env\|canton-localnet.yaml" "$RUNNER_TEMP/canton-localnet/diagnostics"/* 2>/dev/null; then
  echo "FAIL: diagnose.sh output must never name a secret-bearing file's contents"
  failures=$((failures + 1))
else
  echo "ok: diagnose.sh output carries no secret-bearing file's contents"
fi
tests_run=$((tests_run + 1))

## canton_localnet_client_secret_overrides / canton_localnet_client_id_overrides
## (the client-secret/client-id guard boot.sh fails closed on before ever
## booting)

fixed_a_secret="$(canton_localnet_fixed_client_secret "$here/.." A_VALIDATOR_1)"
assert_eq "fixed_client_secret reads a-validator-1's real oauth2.env secret" \
  "AL8648b9SfdTFImq7FV56Vd0KHifHBuC" "$fixed_a_secret"

fixed_a_client_id="$(canton_localnet_fixed_client_id "$here/.." A_VALIDATOR_1)"
assert_eq "fixed_client_id reads a-validator-1's real oauth2.env client id" \
  "a-validator-1-validator" "$fixed_a_client_id"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="   " canton_localnet_client_secret_overrides "$here/..")"
assert_eq "client_secret_overrides: a blank (whitespace-only) override is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="$fixed_a_secret" canton_localnet_client_secret_overrides "$here/..")"
assert_eq "client_secret_overrides: a value equal to the fixed demo secret is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="totally-different-secret" canton_localnet_client_secret_overrides "$here/..")"
assert_eq "client_secret_overrides: a differing value is rejected by name" \
  "CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="  ${fixed_a_secret}  " canton_localnet_client_secret_overrides "$here/..")"
assert_eq "client_secret_overrides: the fixed secret padded with whitespace is rejected (the CLI would export the padded raw value, not the trimmed one)" \
  "CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID="   " canton_localnet_client_id_overrides "$here/..")"
assert_eq "client_id_overrides: a blank (whitespace-only) override is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID="$fixed_a_client_id" canton_localnet_client_id_overrides "$here/..")"
assert_eq "client_id_overrides: a value equal to the fixed demo client id is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID="totally-different-client-id" canton_localnet_client_id_overrides "$here/..")"
assert_eq "client_id_overrides: a differing value is rejected by name" \
  "CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID="  ${fixed_a_client_id}  " canton_localnet_client_id_overrides "$here/..")"
assert_eq "client_id_overrides: the fixed client id padded with whitespace is rejected" \
  "CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_ID" "$got"

## canton_localnet_endpoint_overrides (the URL/host guard boot.sh fails closed
## on alongside client id/secret before ever booting)

got="$(canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: no overrides set — nothing printed" "" "$got"

got="$(CANTON_LOCALNET_HOST="   " canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: blank CANTON_LOCALNET_HOST is not rejected" "" "$got"

got="$(CANTON_LOCALNET_HOST="localhost" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: CANTON_LOCALNET_HOST=localhost is not rejected" "" "$got"

got="$(CANTON_LOCALNET_HOST="127.0.0.1" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: CANTON_LOCALNET_HOST=127.0.0.1 is not rejected" "" "$got"

got="$(CANTON_LOCALNET_HOST="remote.example.com" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: non-localhost CANTON_LOCALNET_HOST is rejected by name" \
  "CANTON_LOCALNET_HOST" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL="   " canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: blank JSON_API_URL is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL="http://localhost:7575" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: localhost JSON_API_URL (self-export value) is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL="http://127.0.0.1:7575" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: 127.0.0.1 JSON_API_URL is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL="https://proxy.example/canton" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: non-localhost JSON_API_URL is rejected by name" \
  "CANTON_LOCALNET_A_VALIDATOR_1_JSON_API_URL" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_TOKEN_URL="http://localhost:8082/realms/a-validator-1/protocol/openid-connect/token" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: localhost TOKEN_URL (self-export value) is not rejected" "" "$got"

got="$(CANTON_LOCALNET_A_VALIDATOR_1_TOKEN_URL="https://external-keycloak.example/token" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: non-localhost TOKEN_URL is rejected by name" \
  "CANTON_LOCALNET_A_VALIDATOR_1_TOKEN_URL" "$got"

got="$(CANTON_LOCALNET_SCAN_URL="http://scan.localhost:10000" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: default scan.localhost SCAN_URL (self-export value) is not rejected" "" "$got"

got="$(CANTON_LOCALNET_SCAN_URL="   " canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: blank SCAN_URL is not rejected" "" "$got"

got="$(CANTON_LOCALNET_SCAN_URL="http://scan.remote.example:10000" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: non-local SCAN_URL is rejected by name" \
  "CANTON_LOCALNET_SCAN_URL" "$got"

got="$(CANTON_LOCALNET_KEYCLOAK_HOST="   " canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: blank CANTON_LOCALNET_KEYCLOAK_HOST is not rejected" "" "$got"

got="$(CANTON_LOCALNET_KEYCLOAK_HOST="localhost" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: CANTON_LOCALNET_KEYCLOAK_HOST=localhost is not rejected" "" "$got"

got="$(CANTON_LOCALNET_KEYCLOAK_HOST="remote-keycloak.example.com" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: non-localhost CANTON_LOCALNET_KEYCLOAK_HOST is rejected by name" \
  "CANTON_LOCALNET_KEYCLOAK_HOST" "$got"

got="$(CANTON_LOCALNET_HOST="remote.example.com" CANTON_LOCALNET_KEYCLOAK_HOST="remote-keycloak.example.com" canton_localnet_endpoint_overrides)"
assert_eq "endpoint_overrides: both HOST and KEYCLOAK_HOST set non-localhost are both rejected" \
  "$(printf 'CANTON_LOCALNET_HOST\nCANTON_LOCALNET_KEYCLOAK_HOST')" "$got"

## canton_localnet_config_has_credentials (the config-file credential guard
## boot.sh fails closed on before ever booting)

config_dir="$(mktemp -d)"

cat >"$config_dir/no-auth.yaml" <<'EOF'
schemaVersion: preview-1
validators:
  a-validator-1:
    partyHint: featuredapp-validator-1
EOF
if canton_localnet_config_has_credentials "$config_dir/no-auth.yaml"; then
  echo "FAIL: config_has_credentials should not match a config with no auth block"
  failures=$((failures + 1))
else
  echo "ok: config_has_credentials does not match a config with no auth block"
fi
tests_run=$((tests_run + 1))

cat >"$config_dir/block-style.yaml" <<'EOF'
validators:
  a-validator-1:
    auth:
      clientId: a-validator-1-validator
      clientSecret: ${FEATUREDAPP_VALIDATOR_SECRET}
EOF
if canton_localnet_config_has_credentials "$config_dir/block-style.yaml"; then
  echo "ok: config_has_credentials matches block-style clientId/clientSecret"
else
  echo "FAIL: config_has_credentials should match block-style clientId/clientSecret"
  failures=$((failures + 1))
fi
tests_run=$((tests_run + 1))

cat >"$config_dir/flow-style.yaml" <<'EOF'
validators:
  a-validator-1:
    partyHint: featuredapp-validator-1
    auth: { clientId: a-validator-1-validator, clientSecret: ${FEATUREDAPP_VALIDATOR_SECRET} }
  c-validator-1: { enabled: false }
EOF
if canton_localnet_config_has_credentials "$config_dir/flow-style.yaml"; then
  echo "ok: config_has_credentials matches README's documented flow-mapping style"
else
  echo "FAIL: config_has_credentials should match README's documented flow-mapping style"
  failures=$((failures + 1))
fi
tests_run=$((tests_run + 1))

cat >"$config_dir/flow-style-secret-only.yaml" <<'EOF'
validators:
  a-validator-1:
    auth: { clientSecret: ${FEATUREDAPP_VALIDATOR_SECRET} }
EOF
if canton_localnet_config_has_credentials "$config_dir/flow-style-secret-only.yaml"; then
  echo "ok: config_has_credentials matches a flow-mapping with clientSecret alone"
else
  echo "FAIL: config_has_credentials should match a flow-mapping with clientSecret alone"
  failures=$((failures + 1))
fi
tests_run=$((tests_run + 1))

cat >"$config_dir/comment-only.yaml" <<'EOF'
# Set validators.a-validator-1.auth.clientId: and clientSecret: via CI secrets,
# never inline in this file.
validators:
  a-validator-1:
    partyHint: featuredapp-validator-1
EOF
if canton_localnet_config_has_credentials "$config_dir/comment-only.yaml"; then
  echo "FAIL: config_has_credentials should not match clientId/clientSecret mentioned only in a comment"
  failures=$((failures + 1))
else
  echo "ok: config_has_credentials does not match clientId/clientSecret mentioned only in a comment"
fi
tests_run=$((tests_run + 1))

rm -rf "$config_dir"

## canton_localnet_multi_sync_active (probes the booted compose project for
## the multi-sync profile's services instead of re-parsing canton-localnet.yaml)

stub_dir="$(mktemp -d)"
cat >"$stub_dir/make" <<'EOF'
#!/usr/bin/env bash
cat <<'OUT'
NAME                     IMAGE     SERVICE            STATUS
multi-sync-startup       busybox   multi-sync-startup exited (0)
multi-sync-ready         busybox   multi-sync-ready   exited (0)
OUT
EOF
chmod +x "$stub_dir/make"
set +e
PATH="$stub_dir:$PATH" canton_localnet_multi_sync_active "$here/.."
status=$?
set -e
assert_status "multi_sync_active: detects multi-sync-ready/-startup in a stubbed 'make status-all'" "0" "$status"
rm -rf "$stub_dir"

stub_dir="$(mktemp -d)"
cat >"$stub_dir/make" <<'EOF'
#!/usr/bin/env bash
cat <<'OUT'
NAME                        IMAGE     SERVICE                  STATUS
a-validator-1-participant   canton    a-validator-1-participant running
OUT
EOF
chmod +x "$stub_dir/make"
set +e
PATH="$stub_dir:$PATH" canton_localnet_multi_sync_active "$here/.."
status=$?
set -e
assert_status "multi_sync_active: absent when the compose project has no multi-sync service" "1" "$status"
rm -rf "$stub_dir"

stub_dir="$(mktemp -d)"
cat >"$stub_dir/make" <<'EOF'
#!/usr/bin/env bash
echo "make: *** No rule to make target 'status-all'." >&2
exit 2
EOF
chmod +x "$stub_dir/make"
set +e
probe_output="$(PATH="$stub_dir:$PATH" canton_localnet_multi_sync_active "$here/.." 2>&1)"
status=$?
set -e
assert_status "multi_sync_active: a failing 'make status-all' reads as not-active" "1" "$status"
assert_eq "multi_sync_active: a failing probe logs a ::warning:: instead of failing silently" \
  "1" "$(grep -c '^::warning::' <<<"$probe_output")"
rm -rf "$stub_dir"

## Run-report helpers: timings, teardown section, unhealthy-container probe

RUNNER_TEMP="$(mktemp -d)"
export RUNNER_TEMP
mkdir -p "$RUNNER_TEMP/canton-localnet"
printf 'CLI resolve (source)\t9\nUp (image pull and compose up)\t120\nWait ready\t41\n' >"$(canton_localnet_timings_file)"
got="$(canton_localnet_render_timings)"
assert_contains "render_timings: lists a phase row" "$got" "| Up (image pull and compose up) | 120 |"
assert_contains "render_timings: sums the phases into a total row" "$got" "| **Total** | **170** |"

: >"$(canton_localnet_timings_file)"
assert_eq "render_timings: prints nothing when no phase was recorded" "" "$(canton_localnet_render_timings)"

stub_dir="$(mktemp -d)"
cat >"$stub_dir/make" <<'EOF'
#!/usr/bin/env bash
cat <<'OUT'
NAME                        IMAGE     SERVICE                     STATUS
a-validator-1-participant   canton    a-validator-1-participant   Up 5 minutes (healthy)
sv-app                      splice    sv-app                      Up 5 minutes (unhealthy)
init-job                    busybox   init-job                    Exited (3) 4 minutes ago
setup-done                  busybox   setup-done                  Exited (0) 4 minutes ago
OUT
EOF
chmod +x "$stub_dir/make"
got="$(PATH="$stub_dir:$PATH" canton_localnet_unhealthy_containers "$here/..")"
assert_eq "unhealthy_containers: lists only unhealthy and non-zero-exit containers" \
  "$(printf 'sv-app\tunhealthy\ninit-job\texited 3')" "$got"

got="$(canton_localnet_render_teardown succeeded "$(printf 'sv-app\tunhealthy')" 'Logs are in the artifact.')"
assert_contains "render_teardown: states the result" "$got" '`down --volumes` succeeded.'
assert_contains "render_teardown: lists the unhealthy container row" "$got" '| `sv-app` | unhealthy |'
assert_contains "render_teardown: points at the logs" "$got" "Logs are in the artifact."

got="$(canton_localnet_render_teardown succeeded '' 'Logs are in the artifact.')"
assert_eq "render_teardown: a clean teardown has no container table or logs pointer" \
  "$(printf '### Teardown\n\n`down --volumes` succeeded.')" "$got"

## down.sh writes the Teardown section, and the summary never carries a secret
## present in the job environment

stub="$(mktemp -d)/canton-localnet"
cat >"$stub" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$stub"
{
  echo "CANTON_LOCALNET_ACTION_CLI_PATH=$stub"
  echo "CANTON_LOCALNET_ACTION_REPO_ROOT=$here/.."
  echo "CANTON_LOCALNET_ACTION_CONFIG_PATH=/dev/null"
} >"$(canton_localnet_state_file)"
summary="$(mktemp)"
out="$(mktemp)"
bash "$here/host-diagnostics.sh" start-sampler "$(canton_localnet_memory_samples_file)"
sampler_pid="$(cat "$(canton_localnet_memory_samples_file).pid")"
set +e
PATH="$stub_dir:$PATH" STRICT=true GITHUB_OUTPUT="$out" GITHUB_STEP_SUMMARY="$summary" GITHUB_JOB=probe \
  CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="TEARDOWN-SYNTHETIC-SECRET-41d8" \
  bash "$here/down.sh" >/dev/null 2>&1
status=$?
set -e
assert_status "down.sh (strict): a succeeding stub 'down' exits 0 with unhealthy containers present" "0" "$status"
assert_contains "down.sh (strict): summary has the Teardown section" "$(cat "$summary")" '`down --volumes` succeeded.'
assert_contains "down.sh (strict): summary lists the unhealthy container" "$(cat "$summary")" '| `sv-app` | unhealthy |'
assert_contains "down.sh (strict): summary names the teardown diagnostics artifact" "$(cat "$summary")" "canton-localnet-teardown-diagnostics-probe"
assert_contains "down.sh (strict): unhealthy containers set unhealthy=true" "$(cat "$out")" "unhealthy=true"
for _ in 1 2 3 4 5; do
  kill -0 "$sampler_pid" 2>/dev/null || break
  sleep 1
done
if kill -0 "$sampler_pid" 2>/dev/null || [ -e "$(canton_localnet_memory_samples_file).pid" ]; then
  echo "FAIL: down.sh should stop the memory sampler boot.sh started"
  kill "$sampler_pid" 2>/dev/null || true
  failures=$((failures + 1))
else
  echo "ok: down.sh stops the memory sampler boot.sh started"
fi
tests_run=$((tests_run + 1))
tests_run=$((tests_run + 1))
if grep -qF "TEARDOWN-SYNTHETIC-SECRET-41d8" "$summary"; then
  echo "FAIL: down.sh's summary contains a secret from the job environment"
  failures=$((failures + 1))
else
  echo "ok: down.sh's summary contains no secret from the job environment"
fi
rm -rf "$stub_dir"

## T4: `canton-localnet env --format github` masks a synthetic secret set
## through the documented per-slot override (CANTON_LOCALNET_A_VALIDATOR_1_
## CLIENT_SECRET), before it ever writes $GITHUB_ENV. Needs a real build of
## the CLI — action-selftest.yaml's T1 job sets CANTON_LOCALNET_BIN; a bare
## local run of this script skips it rather than failing.

if [ -n "${CANTON_LOCALNET_BIN:-}" ]; then
  probe_secret="SELFTEST-SYNTHETIC-SECRET-9f3c7e21"
  probe_env_file="$(mktemp)"
  probe_out="$(CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="$probe_secret" \
    GITHUB_ENV="$probe_env_file" \
    "$CANTON_LOCALNET_BIN" env --format github --slot a --repo-root "$here/.." 2>&1)"

  tests_run=$((tests_run + 1))
  if grep -qF "::add-mask::${probe_secret}" <<<"$probe_out"; then
    echo "ok: env --format github emits ::add-mask:: for the synthetic client-secret override"
  else
    echo "FAIL: expected ::add-mask::${probe_secret} in env --format github's own stdout"
    echo "--- actual stdout ---"
    echo "$probe_out"
    failures=$((failures + 1))
  fi

  tests_run=$((tests_run + 1))
  if grep -vF "::add-mask::${probe_secret}" <<<"$probe_out" | grep -qF "$probe_secret"; then
    echo "FAIL: the raw synthetic secret appears somewhere in stdout outside its own ::add-mask:: line"
    failures=$((failures + 1))
  else
    echo "ok: the raw synthetic secret appears only on its own ::add-mask:: line, nowhere else in stdout"
  fi

  summary_secret="SUMMARY-SYNTHETIC-SECRET-77ab"
  summary_pqs_password="SUMMARY-SYNTHETIC-PQS-PASSWORD-52e1"
  summary_file="$(mktemp)"
  CANTON_LOCALNET_A_VALIDATOR_1_CLIENT_SECRET="$summary_secret" \
    PQS_A_VALIDATOR_1_READER_PASSWORD="$summary_pqs_password" \
    GITHUB_ENV="$(mktemp)" \
    "$CANTON_LOCALNET_BIN" env --format github --pqs --slot a --summary-file "$summary_file" --repo-root "$here/.." >/dev/null 2>&1
  summary_text="$(cat "$summary_file")"
  assert_contains "env --summary-file: validators table row for a-validator-1" "$summary_text" '| `a-validator-1` | http://localhost:11975 | http://localhost:11901 | http://localhost:11902 | http://localhost:11903 | yes |'
  for forbidden in "$summary_secret" "$summary_pqs_password" "Password="; do
    tests_run=$((tests_run + 1))
    if grep -qF "$forbidden" <<<"$summary_text"; then
      echo "FAIL: env --summary-file output contains [$forbidden]"
      failures=$((failures + 1))
    else
      echo "ok: env --summary-file output does not contain [$forbidden]"
    fi
  done
else
  echo "skip: CANTON_LOCALNET_BIN not set — action-selftest.yaml's T1 job builds the CLI and sets it; run this script from there to exercise the masking assertion"
fi

## boot.sh passes the selected config to the PQS watermark wait

boot_dir="$(mktemp -d)"
boot_argv_log="$boot_dir/argv.log"
cat >"$boot_dir/canton-localnet" <<'EOF'
#!/usr/bin/env bash
echo "$*" >>"$BOOT_ARGV_LOG"
EOF
cat >"$boot_dir/make" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$boot_dir/timeout" <<'EOF'
#!/usr/bin/env bash
shift
exec "$@"
EOF
chmod +x "$boot_dir/canton-localnet" "$boot_dir/make" "$boot_dir/timeout"
mkdir -p "$boot_dir/runner-temp"
echo "validators: {}" >"$boot_dir/ci-specific.yaml"
: >"$boot_dir/step-summary"
: >"$boot_dir/github-output"
PATH="$boot_dir:$PATH" BOOT_ARGV_LOG="$boot_argv_log" \
  CLI_PATH="$boot_dir/canton-localnet" ACTION_PATH="$here/.." VALIDATORS=a PQS=true \
  OBSERVABILITY=false MULTI_SYNC=false DIALECT=native ROLES="" JWT=false TIMEOUT=10m \
  CONFIG_INPUT="$boot_dir/ci-specific.yaml" RUNNER_TEMP="$boot_dir/runner-temp" \
  RUNNER_ENVIRONMENT=github-hosted GITHUB_WORKSPACE="$boot_dir" \
  GITHUB_OUTPUT="$boot_dir/github-output" GITHUB_STEP_SUMMARY="$boot_dir/step-summary" \
  bash "$here/boot.sh" >/dev/null 2>&1
status=$?
assert_status "boot.sh (pqs: true): runs to completion against a stub CLI" "0" "$status"
pqs_wait_line="$(grep -- '--pqs' "$boot_argv_log" | grep '^wait-ready' || true)"
assert_contains "boot.sh (pqs: true): the PQS wait-ready call is recorded" "$pqs_wait_line" "--slot a-validator-1"
assert_contains "boot.sh (pqs: true): the PQS wait-ready call carries the selected --config" "$pqs_wait_line" "--config $boot_dir/ci-specific.yaml"
bash "$here/host-diagnostics.sh" stop-sampler "$boot_dir/runner-temp/canton-localnet/memory-samples.txt"
rm -rf "$boot_dir"

## boot.sh caps each up attempt at up-timeout, and captures host diagnostics when the first attempt fails

boot_dir="$(mktemp -d)"
cat >"$boot_dir/canton-localnet" <<'EOF'
#!/usr/bin/env bash
if [ "$1" = "up" ]; then
  echo "up" >>"$BOOT_UP_LOG"
  [ "$(wc -l <"$BOOT_UP_LOG")" -gt "$BOOT_FAILING_UP_ATTEMPTS" ] || exit "$BOOT_FAILING_UP_EXIT"
fi
exit 0
EOF
cat >"$boot_dir/make" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$boot_dir/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$boot_dir/timeout" <<'EOF'
#!/usr/bin/env bash
echo "$1" >>"$BOOT_TIMEOUT_LOG"
shift
exec "$@"
EOF
chmod +x "$boot_dir/canton-localnet" "$boot_dir/make" "$boot_dir/sleep" "$boot_dir/timeout"

run_boot_with_up_timeout() {
  local up_timeout="$1" failing_up_attempts="$2" failing_up_exit="$3"
  boot_runner_temp="$(mktemp -d)"
  : >"$boot_dir/up.log"
  : >"$boot_dir/timeout.log"
  : >"$boot_dir/step-summary"
  : >"$boot_dir/github-output"
  set +e
  PATH="$boot_dir:$PATH" BOOT_UP_LOG="$boot_dir/up.log" BOOT_TIMEOUT_LOG="$boot_dir/timeout.log" \
    BOOT_FAILING_UP_ATTEMPTS="$failing_up_attempts" BOOT_FAILING_UP_EXIT="$failing_up_exit" \
    CLI_PATH="$boot_dir/canton-localnet" ACTION_PATH="$here/.." VALIDATORS=a PQS=false \
    OBSERVABILITY=false MULTI_SYNC=false DIALECT=native ROLES="" JWT=false TIMEOUT=10m UP_TIMEOUT="$up_timeout" \
    CONFIG_INPUT="" RUNNER_TEMP="$boot_runner_temp" \
    RUNNER_ENVIRONMENT=github-hosted GITHUB_WORKSPACE="$boot_dir" \
    GITHUB_OUTPUT="$boot_dir/github-output" GITHUB_STEP_SUMMARY="$boot_dir/step-summary" \
    bash "$here/boot.sh" >"$boot_dir/boot.log" 2>&1
  status=$?
  set -e
  bash "$here/host-diagnostics.sh" stop-sampler "$boot_runner_temp/canton-localnet/memory-samples.txt"
}

up_attempt_caps_within_total_deadline() {
  grep -E '^[0-9]+s$' "$boot_dir/timeout.log" | while IFS= read -r cap; do
    if [ "${cap%s}" -ge 570 ] && [ "${cap%s}" -le 600 ]; then
      echo "within the 10m total deadline"
    else
      echo "$cap"
    fi
  done | sort -u
}

run_boot_with_up_timeout 2m 1 1
assert_status "boot.sh (up-timeout): a failed first up is retried and the boot completes" "0" "$status"
assert_eq "boot.sh (up-timeout): both up attempts were capped at up-timeout" "2" "$(grep -c '^120s$' "$boot_dir/timeout.log")"
if [ -d "$boot_runner_temp/canton-localnet/diagnostics/host-attempt1" ]; then
  echo "ok: boot.sh (up-timeout): host diagnostics were captured after the first failed attempt"
else
  echo "FAIL: boot.sh (up-timeout): expected host-attempt1 diagnostics"
  failures=$((failures + 1))
fi
tests_run=$((tests_run + 1))

run_boot_with_up_timeout 2m 2 124
assert_status "boot.sh (up-timeout): a retry that also times out fails the boot with timeout's exit 124" "124" "$status"
assert_eq "boot.sh (up-timeout): a retry that also times out ran exactly two up attempts" "2" "$(wc -l <"$boot_dir/up.log" | tr -d ' ')"

for zero_cap in 0s 0m 0h; do
  run_boot_with_up_timeout "$zero_cap" 1 1
  assert_status "boot.sh (up-timeout: $zero_cap): a zero cap boots like an empty one" "0" "$status"
  assert_eq "boot.sh (up-timeout: $zero_cap): every up attempt stays inside the total timeout instead of running unbounded" \
    "within the 10m total deadline" "$(up_attempt_caps_within_total_deadline)"
done

run_boot_with_up_timeout '' 1 1
assert_status "boot.sh (up-timeout empty): a failed first up is retried and the boot completes" "0" "$status"
assert_eq "boot.sh (up-timeout empty): every up attempt stays inside the total timeout" \
  "within the 10m total deadline" "$(up_attempt_caps_within_total_deadline)"

for malformed_cap in bogus -1m 15 1d; do
  run_boot_with_up_timeout "$malformed_cap" 0 1
  if [ "$status" -ne 0 ]; then
    echo "ok: boot.sh (up-timeout: $malformed_cap): a malformed cap fails the boot"
  else
    echo "FAIL: boot.sh (up-timeout: $malformed_cap): a malformed cap should fail the boot"
    failures=$((failures + 1))
  fi
  tests_run=$((tests_run + 1))
  assert_contains "boot.sh (up-timeout: $malformed_cap): the error names the up-timeout input" "$(cat "$boot_dir/boot.log")" "'up-timeout' must look like"
  assert_eq "boot.sh (up-timeout: $malformed_cap): no up attempt runs" "0" "$(wc -l <"$boot_dir/up.log" | tr -d ' ')"
done
rm -rf "$boot_dir"

## host-diagnostics.sh captures what a memory-starvation post-mortem needs, and never fails the caller

diag_dir="$(mktemp -d)"
mkdir -p "$diag_dir/bin"
cat >"$diag_dir/bin/docker" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"label=com.docker.compose.project=localnet"*) containers='splice canton postgres' ;;
  *"label=com.docker.compose.project="*) containers='' ;;
  *) containers='splice canton postgres foreign-splice-db' ;;
esac
case "$1" in
  ps)
    case "$*" in
      *"{{.Names}}"*) [ -z "$containers" ] || printf '%s\n' $containers ;;
      *) echo "NAMES $containers" ;;
    esac
    ;;
  logs) echo "stub log for ${*: -1}" ;;
  stats)
    shift 2
    if [ "$#" -eq 0 ]; then
      echo "stub stats for every container on the daemon: $containers"
    else
      echo "stub stats for $*"
    fi
    ;;
  inspect) shift 3; echo "stub inspect for $*" ;;
esac
EOF
cat >"$diag_dir/bin/free" <<'EOF'
#!/usr/bin/env bash
echo "stub free"
EOF
chmod +x "$diag_dir/bin/docker" "$diag_dir/bin/free"
printf '100 3000 4000\n110 7600 100\n120 5000 2700\n' >"$diag_dir/samples.txt"
set +e
PATH="$diag_dir/bin:$PATH" bash "$here/host-diagnostics.sh" capture "$diag_dir/out" "$diag_dir/samples.txt" >"$diag_dir/capture.log" 2>&1
status=$?
set -e
assert_status "host-diagnostics.sh: capture exits 0" "0" "$status"
assert_contains "host-diagnostics.sh: free -m output is captured" "$(cat "$diag_dir/out/free-m.txt")" "stub free"
assert_contains "host-diagnostics.sh: peak memory is the highest used sample" "$(cat "$diag_dir/out/memory-peak.txt")" "peak used 7600 MiB"
assert_contains "host-diagnostics.sh: lowest available memory is reported" "$(cat "$diag_dir/out/memory-peak.txt")" "lowest available 100 MiB"
assert_contains "host-diagnostics.sh: splice logs are captured" "$(cat "$diag_dir/out/logs-splice.txt")" "stub log for splice"
assert_contains "host-diagnostics.sh: canton logs are captured" "$(cat "$diag_dir/out/logs-canton.txt")" "stub log for canton"
if [ -e "$diag_dir/out/logs-postgres.txt" ]; then
  echo "FAIL: host-diagnostics.sh: only splice and canton logs should be captured"
  failures=$((failures + 1))
else
  echo "ok: host-diagnostics.sh: only splice and canton logs are captured"
fi
tests_run=$((tests_run + 1))
assert_contains "host-diagnostics.sh: container health is captured" "$(cat "$diag_dir/out/container-health.txt")" "stub inspect"
assert_contains "host-diagnostics.sh: nproc is captured" "$(ls "$diag_dir/out")" "nproc.txt"
assert_contains "host-diagnostics.sh: the kernel OOM scan is captured" "$(ls "$diag_dir/out")" "dmesg-oom.txt"
assert_contains "host-diagnostics.sh: docker stats are captured" "$(cat "$diag_dir/out/docker-stats.txt")" "stub stats for splice canton postgres"
if grep -rqF "foreign-splice-db" "$diag_dir/out"; then
  echo "FAIL: host-diagnostics.sh: a container outside the LocalNet compose project leaked into the capture: $(grep -rlF foreign-splice-db "$diag_dir/out" | tr '\n' ' ')"
  failures=$((failures + 1))
else
  echo "ok: host-diagnostics.sh: a splice-named container outside the LocalNet compose project is not captured"
fi
tests_run=$((tests_run + 1))

set +e
PATH="$diag_dir/bin:$PATH" COMPOSE_PROJECT_NAME=slot2 bash "$here/host-diagnostics.sh" capture "$diag_dir/out-slot2" >/dev/null 2>&1
status=$?
set -e
assert_status "host-diagnostics.sh (COMPOSE_PROJECT_NAME=slot2): capture exits 0" "0" "$status"
assert_contains "host-diagnostics.sh (COMPOSE_PROJECT_NAME=slot2): the scope names the project" "$(cat "$diag_dir/out-slot2/container-scope.txt")" "compose project: slot2"
assert_eq "host-diagnostics.sh (COMPOSE_PROJECT_NAME=slot2): an empty project captures no stats, health or logs, never the whole daemon" \
  "container-scope.txt dmesg-oom.txt docker-ps.txt free-m.txt nproc.txt" "$(ls "$diag_dir/out-slot2" | LC_ALL=C sort | tr '\n' ' ' | sed 's/ $//')"

set +e
PATH="$diag_dir/bin:$PATH" COMPOSE_PROJECT_NAME='Not A Project' bash "$here/host-diagnostics.sh" capture "$diag_dir/out-invalid" >/dev/null 2>&1
status=$?
set -e
assert_status "host-diagnostics.sh (invalid COMPOSE_PROJECT_NAME): capture exits 0" "0" "$status"
assert_contains "host-diagnostics.sh (invalid COMPOSE_PROJECT_NAME): container capture is skipped" "$(cat "$diag_dir/out-invalid/container-scope.txt")" "skipped"
assert_eq "host-diagnostics.sh (invalid COMPOSE_PROJECT_NAME): no container is listed, inspected or logged" \
  "container-scope.txt dmesg-oom.txt free-m.txt nproc.txt" "$(ls "$diag_dir/out-invalid" | LC_ALL=C sort | tr '\n' ' ' | sed 's/ $//')"
rm -rf "$diag_dir"

echo
echo "$tests_run tests run, $failures failed"
if [ "$failures" -ne 0 ]; then
  exit 1
fi
