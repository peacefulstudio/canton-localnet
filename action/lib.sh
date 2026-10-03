# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
# shellcheck shell=bash
#
# Shared shell helpers for action/*.sh and teardown/../action/*.sh. Sourced,
# never executed directly. Every function here is pure bash/coreutils so the
# action needs no extra runner dependency beyond what setup-cli already
# requires. This file intentionally sets none of bash's -e/-u/-o pipefail:
# the sourcing script owns its own error-handling mode (diagnose.sh, for
# one, wants every collection command to be best-effort).

canton_localnet_state_file() {
  echo "${RUNNER_TEMP:?RUNNER_TEMP is required}/canton-localnet/state.env"
}

canton_localnet_log_warning() {
  echo "::warning::$1"
}

canton_localnet_log_error() {
  echo "::error::$1" >&2
}

# canton_localnet_has_leftover_containers reports (via exit status) whether
# a `docker compose down` log names a container it actually removed or
# stopped. Compose ends each status line with the verb (e.g.
# "Container foo  Removed"), not trailing whitespace, so the verb match
# must accept end-of-line as well as a following space.
canton_localnet_has_leftover_containers() {
  grep -qE "(^|[[:space:]])(Removing|Stopping|Removed|Stopped)($|[[:space:]])" <<<"$1"
}

# canton_localnet_multi_sync_active reports (via exit status) whether the
# multi-sync profile's services exist in the compose project at repo_root,
# by grepping `make -C repo_root status-all` — the same compose-file and
# profile layering the top-level Makefile uses, so it stays project-scoped
# in step with `make status-all` rather than re-deriving it — for the
# multi-sync-only service names declared in
# compose/modules/localnet/compose.yaml (multi-sync-startup,
# multi-sync-ready). This probes what actually booted instead of
# re-parsing canton-localnet.yaml in bash, so a truthy multiSync the CLI's
# own yaml.v3 parser accepts but a bash regex would miss (flow mappings,
# `multiSync : true`, ...) is still detected. A `make`/`docker` error
# reads as "not active" but is not silent: it logs a `::warning::` naming
# the probe failure before returning, so a broken probe surfaces instead
# of masquerading as a single-synchronizer stack.
canton_localnet_multi_sync_active() {
  local output
  if ! output="$(make -C "$1" status-all 2>/dev/null)"; then
    canton_localnet_log_warning "canton-localnet action: 'make status-all' failed while probing for the multi-sync profile in $1; treating multi-sync as not active"
    return 1
  fi
  grep -qE 'multi-sync-startup|multi-sync-ready' <<<"$output"
}

# canton_localnet_slot_prefix prints the two-digit host-port prefix for a
# short slot letter (sv|a|b|c|d), matching cli/internal/slot/slot.go's
# PortPrefix table. Exits non-zero on an unknown slot.
canton_localnet_slot_prefix() {
  case "$1" in
    sv) echo "10" ;;
    a) echo "11" ;;
    b) echo "12" ;;
    c) echo "13" ;;
    d) echo "14" ;;
    *)
      canton_localnet_log_error "canton-localnet action: unknown validator slot '$1' (known: sv, a, b, c, d)"
      return 1
      ;;
  esac
}

# canton_localnet_slot_canonical prints the canonical slot name for a short
# letter (sv|a|b|c|d), matching slot.go's Canonical field.
canton_localnet_slot_canonical() {
  case "$1" in
    sv) echo "sv-validator-1" ;;
    a) echo "a-validator-1" ;;
    b) echo "b-validator-1" ;;
    c) echo "c-validator-1" ;;
    d) echo "d-validator-1" ;;
    *)
      canton_localnet_log_error "canton-localnet action: unknown validator slot '$1' (known: sv, a, b, c, d)"
      return 1
      ;;
  esac
}

# canton_localnet_trim_space prints value with leading and trailing
# whitespace removed, matching Go's strings.TrimSpace as used by the CLI's
# own EnvIsSet/pick (cli/internal/slot/resolve.go) — the "set" definition
# a caller comparing against a CANTON_LOCALNET_*_CLIENT_SECRET override
# must share with the CLI to avoid rejecting a value the CLI would accept.
canton_localnet_trim_space() {
  local v="$1"
  v="${v#"${v%%[![:space:]]*}"}"
  v="${v%"${v##*[![:space:]]}"}"
  echo "$v"
}

# canton_localnet_env_file_value prints the value of key in a KEY=VALUE env
# file, stripping a trailing inline comment (a '#' preceded by
# start-of-value or whitespace, never mid-token) and surrounding whitespace
# like the CLI's own env-file parser
# (cli/internal/slot/resolve.go:ReadEnvFile/stripInlineComment) — every
# oauth2.env carries a trailing "# validator client"-style comment on its
# CLIENT_ID line, so skipping this step would compare against the
# untouched comment text instead of the real value. Unlike the CLI's
# parser, this does not strip a layer of paired quotes around the value;
# no oauth2.env fixture quotes its values, so this has not mattered in
# practice. Prints nothing on a missing file or key, without failing.
canton_localnet_env_file_value() {
  local env_file="$1" key="$2" raw
  [ -f "$env_file" ] || return 0
  raw="$(grep -E "^${key}=" "$env_file" | tail -n1 | cut -d= -f2-)"
  [ -z "$raw" ] && return 0
  canton_localnet_trim_space "$(sed -E 's/(^|[[:space:]])#.*$//' <<<"$raw")"
}

# canton_localnet_fixed_client_secret prints the fixed LocalNet demo OAuth2
# client secret this action's booted Keycloak uses for env-prefix (e.g.
# "A_VALIDATOR_1", matching cli/internal/slot.Slot.EnvPrefix()), read from
# compose/modules/keycloak/env/<slot>/on/oauth2.env under repo_root. Prints
# nothing, without failing, when the slot or its oauth2.env doesn't exist —
# callers must treat an empty result as "no fixed secret to compare
# against", not as the empty string being the fixed secret.
canton_localnet_fixed_client_secret() {
  local repo_root="$1" env_prefix="$2" slot
  slot="$(tr '[:upper:]' '[:lower:]' <<<"$env_prefix" | tr '_' '-')"
  canton_localnet_env_file_value \
    "$repo_root/compose/modules/keycloak/env/$slot/on/oauth2.env" \
    "AUTH_${env_prefix}_VALIDATOR_CLIENT_SECRET"
}

# canton_localnet_fixed_client_id prints the fixed LocalNet demo OAuth2
# client id this action's booted Keycloak uses for env-prefix, read from
# compose/modules/keycloak/env/<slot>/on/oauth2.env under repo_root — same
# shape and same "nothing means no fixed id to compare against" contract as
# canton_localnet_fixed_client_secret.
canton_localnet_fixed_client_id() {
  local repo_root="$1" env_prefix="$2" slot
  slot="$(tr '[:upper:]' '[:lower:]' <<<"$env_prefix" | tr '_' '-')"
  canton_localnet_env_file_value \
    "$repo_root/compose/modules/keycloak/env/$slot/on/oauth2.env" \
    "AUTH_${env_prefix}_VALIDATOR_CLIENT_ID"
}

# canton_localnet_client_secret_overrides prints, one per line, the names
# of CANTON_LOCALNET_<SLOT>_CLIENT_SECRET env vars in the current process
# environment that are both set (per canton_localnet_trim_space) and
# different from the fixed demo secret action_path's booted Keycloak uses
# for that slot (per canton_localnet_fixed_client_secret) — the set boot.sh
# must fail closed on. Prints nothing when there is nothing to reject,
# including when a var is blank or already equal to the fixed secret (for
# example because a prior boot in the same job already exported it into
# $GITHUB_ENV). The comparison against the fixed secret uses the raw,
# untrimmed value — matching the CLI's own slot.pick, which returns the env
# var verbatim once EnvIsSet's trimmed-non-blank check passes — so a value
# that only equals the fixed secret after trimming still counts as a
# differing override and is rejected. Never fails.
canton_localnet_client_secret_overrides() {
  local action_path="$1" name value trimmed env_prefix fixed_secret
  while IFS= read -r name; do
    value="${!name}"
    trimmed="$(canton_localnet_trim_space "$value")"
    [ -z "$trimmed" ] && continue
    env_prefix="${name#CANTON_LOCALNET_}"
    env_prefix="${env_prefix%_CLIENT_SECRET}"
    fixed_secret="$(canton_localnet_fixed_client_secret "$action_path" "$env_prefix")"
    if [ -n "$fixed_secret" ] && [ "$value" = "$fixed_secret" ]; then
      continue
    fi
    echo "$name"
  done < <(compgen -v | grep -E '^CANTON_LOCALNET_[A-Z0-9_]+_VALIDATOR_[0-9]+_CLIENT_SECRET$' || true)
}

# canton_localnet_client_id_overrides is canton_localnet_client_secret_overrides's
# strict counterpart for CANTON_LOCALNET_<SLOT>_CLIENT_ID: slot.Resolve gives
# that env var the same precedence over AUTH_<SLOT>_VALIDATOR_CLIENT_ID that
# it gives CLIENT_SECRET over AUTH_<SLOT>_VALIDATOR_CLIENT_SECRET, so an
# override that doesn't match this action's booted Keycloak client id is the
# same kind of silent breakage. Same rules: blank-after-trim is unset, the
# raw untrimmed value must equal the fixed id exactly, never fails.
canton_localnet_client_id_overrides() {
  local action_path="$1" name value trimmed env_prefix fixed_id
  while IFS= read -r name; do
    value="${!name}"
    trimmed="$(canton_localnet_trim_space "$value")"
    [ -z "$trimmed" ] && continue
    env_prefix="${name#CANTON_LOCALNET_}"
    env_prefix="${env_prefix%_CLIENT_ID}"
    fixed_id="$(canton_localnet_fixed_client_id "$action_path" "$env_prefix")"
    if [ -n "$fixed_id" ] && [ "$value" = "$fixed_id" ]; then
      continue
    fi
    echo "$name"
  done < <(compgen -v | grep -E '^CANTON_LOCALNET_[A-Z0-9_]+_VALIDATOR_[0-9]+_CLIENT_ID$' || true)
}

# canton_localnet_endpoint_overrides prints, one per line, the names of
# CANTON_LOCALNET_HOST, CANTON_LOCALNET_KEYCLOAK_HOST, CANTON_LOCALNET_SCAN_URL and per-slot
# CANTON_LOCALNET_<SLOT>_{JSON_API_URL, TOKEN_URL} vars that are both set
# (non-blank after trimming) and do not resolve to localhost — the set
# boot.sh must fail closed on. slot.Resolve gives these vars precedence over
# its computed defaults, so a non-localhost value in the process environment
# causes `canton-localnet env` to export URLs that point at a different
# stack than the one this action booted. CANTON_LOCALNET_KEYCLOAK_HOST needs
# its own check alongside CANTON_LOCALNET_HOST: resolve.go defaults it to
# CANTON_LOCALNET_HOST but a caller can set it independently, and it alone
# feeds KeycloakHostBase, which becomes TokenURLHost whenever no per-slot
# _TOKEN_URL override is set. Values written back by a prior
# `canton-localnet env --format github` run in the same job always start
# with "http://localhost:" and pass the check, so a second use of this
# action in the same job is never rejected. Never fails.
canton_localnet_endpoint_overrides() {
  local name value trimmed
  for name in CANTON_LOCALNET_HOST CANTON_LOCALNET_KEYCLOAK_HOST; do
    value="${!name:-}"
    trimmed="$(canton_localnet_trim_space "$value")"
    if [ -n "$trimmed" ] && [ "$trimmed" != "localhost" ] && [ "$trimmed" != "127.0.0.1" ]; then
      echo "$name"
    fi
  done
  trimmed="$(canton_localnet_trim_space "${CANTON_LOCALNET_SCAN_URL:-}")"
  if [ -n "$trimmed" ]; then
    case "$trimmed" in
      http://scan.localhost:* | http://localhost:* | http://127.0.0.1:*) ;;
      *) echo "CANTON_LOCALNET_SCAN_URL" ;;
    esac
  fi
  while IFS= read -r name; do
    value="${!name}"
    trimmed="$(canton_localnet_trim_space "$value")"
    [ -z "$trimmed" ] && continue
    case "$trimmed" in
      http://localhost:* | http://127.0.0.1:*) continue ;;
    esac
    echo "$name"
  done < <(compgen -v | grep -E '^CANTON_LOCALNET_[A-Z0-9_]+_VALIDATOR_[0-9]+_(JSON_API_URL|TOKEN_URL)$' || true)
}

# canton_localnet_config_has_credentials reports (via exit status) whether
# the YAML config at path sets validators.*.auth.clientId or clientSecret —
# the set boot.sh must fail closed on, since this action's booted Keycloak
# cannot be reconfigured at boot time. Matches both the block style
# ("      clientId: foo") and the flow-mapping style the repo's own
# README.md documents ("auth: { clientId: ..., clientSecret: ... }"): the
# key must be preceded by start-of-line, '{', ',' or whitespace, so a key
# that merely contains "clientId"/"clientSecret" as a substring doesn't
# count. Strips '#' comments first with the same start-of-value-or-preceding-
# whitespace rule as canton_localnet_env_file_value, so a comment that
# mentions clientId/clientSecret by name isn't mistaken for a real key. Not
# a full YAML parser — a quoted key ('"clientId":') or a multi-line flow
# value isn't caught; a planned CLI-side resolved-config check would
# replace this with an exact match against the parsed config. The sed
# output is captured into a variable rather than piped straight into `grep
# -q`, so a match early in a large config can't make `grep` exit before
# `sed` finishes writing, which under a caller's `pipefail` would surface
# `sed`'s SIGPIPE as this function's exit status instead of the match.
canton_localnet_config_has_credentials() {
  local stripped
  stripped="$(sed -E 's/(^|[[:space:]])#.*$//' "$1")"
  grep -qE '(^|[{,[:space:]])(clientId|clientSecret)[[:space:]]*:[[:space:]]*[^[:space:]]' <<<"$stripped"
}

# canton_localnet_normalize_validators splits a comma- and/or space-separated
# list of a|b|c|d (or their canonical <x>-validator-1 form) into a
# deduplicated, order-preserving space-separated list of short letters.
# sv is rejected here: it is never toggled through this input, it is always
# on. An empty or all-blank input is also rejected — the action.yml default
# ('a') is what callers see if they pass nothing.
canton_localnet_normalize_validators() {
  local raw="$1" token normalized out=() seen=""
  raw="${raw//,/ }"
  for token in $raw; do
    normalized="$(echo "$token" | tr '[:upper:]' '[:lower:]')"
    case "$normalized" in
      a | a-validator-1) normalized="a" ;;
      b | b-validator-1) normalized="b" ;;
      c | c-validator-1) normalized="c" ;;
      d | d-validator-1) normalized="d" ;;
      sv | sv-validator-1)
        canton_localnet_log_error "canton-localnet action: 'validators' cannot include sv — the SV slot is always on"
        return 1
        ;;
      *)
        canton_localnet_log_error "canton-localnet action: unknown entry '$token' in 'validators' (want a, b, c, d)"
        return 1
        ;;
    esac
    case " $seen " in
      *" $normalized "*) continue ;;
    esac
    seen="$seen $normalized"
    out+=("$normalized")
  done
  if [ "${#out[@]}" -eq 0 ]; then
    canton_localnet_log_error "canton-localnet action: 'validators' resolved to no slots"
    return 1
  fi
  echo "${out[@]}"
}

# canton_localnet_deadline_remaining prints the whole seconds left before
# deadline_epoch (a unix timestamp), or fails with phase in the message if
# the deadline has already passed. It backs one total deadline shared by
# pre-boot cleanup, up-with-retry and wait-ready, not a fresh per-slot
# timeout.
canton_localnet_deadline_remaining() {
  local deadline_epoch="$1" phase="$2" now remaining
  now="$(date +%s)"
  remaining=$((deadline_epoch - now))
  if [ "$remaining" -le 0 ]; then
    canton_localnet_log_error "canton-localnet action: total 'timeout' budget was already spent before ${phase}"
    return 1
  fi
  echo "$remaining"
}

# canton_localnet_parse_duration converts a Go-duration-like string
# (e.g. "15m", "90s", "1h") into whole seconds. Only a single numeric
# component with one of s/m/h is accepted — the same subset the CLI's own
# --timeout flags parse via time.ParseDuration, restricted to what the
# action's own inputs realistically need.
canton_localnet_parse_duration() {
  local raw="$1" value unit
  if [[ "$raw" =~ ^([0-9]+)(s|m|h)$ ]]; then
    value="${BASH_REMATCH[1]}"
    unit="${BASH_REMATCH[2]}"
    case "$unit" in
      s) echo "$value" ;;
      m) echo "$((value * 60))" ;;
      h) echo "$((value * 3600))" ;;
    esac
    return 0
  fi
  canton_localnet_log_error "canton-localnet action: 'timeout' must look like 15m, 900s or 1h — got '$raw'"
  return 1
}
