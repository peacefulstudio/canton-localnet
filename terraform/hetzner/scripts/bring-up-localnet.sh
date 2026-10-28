#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

EXIT_FAILED=1
EXIT_BROKEN=2

usage() {
  cat <<EOF
Usage: $0 <checkout-dir> <state-key-file> <up-command>...

Bring LocalNet up from <checkout-dir> on a box whose Docker volumes persist
across server rebuilds, wiping those volumes when the LocalNet version that
wrote them differs from the one about to run.

The state key names what invalidates on-disk state: SPLICE_VERSION (which pins
the Canton image), POSTGRES_VERSION (the Postgres major) and
CANTON_PROTOCOL_VERSION. Each is read from the environment, else from
compose/.env.defaults or compose/modules/localnet/compose.env.

  - <state-key-file> missing or different: make clean (down with volumes);
  - otherwise: make down (containers only, volumes kept).

Then <up-command> runs, and only when it succeeds is the current key written
to <state-key-file>.

Exit codes: ${EXIT_FAILED} a step failed, ${EXIT_BROKEN} the script could not run.
EOF
}

case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac

if [ "$#" -lt 3 ]; then
  usage >&2
  exit "${EXIT_BROKEN}"
fi

checkout="$1"
key_file="$2"
shift 2

log() {
  echo "bring-up-localnet: $*"
}

broken() {
  echo "bring-up-localnet: FATAL: $*" >&2
  exit "${EXIT_BROKEN}"
}

failed() {
  echo "bring-up-localnet: FATAL: $*" >&2
  exit "${EXIT_FAILED}"
}

env_file_value() {
  local name="$1" file="$2" value
  value="$(sed -n "s/^${name}=//p" "$file" | tail -n 1)"
  local defaulted_pattern="^\\$\\{${name}:-([^}]*)\\}$"
  if [[ "$value" =~ $defaulted_pattern ]]; then
    value="${BASH_REMATCH[1]}"
  fi
  printf '%s' "$value"
}

setting() {
  local name="$1" file="$2" value
  value="${!name:-}"
  if [ -z "$value" ]; then
    [ -f "$file" ] || broken "$file not found while resolving $name"
    value="$(env_file_value "$name" "$file")"
  fi
  if [ -z "$value" ] || [[ "$value" == *'$'* ]]; then
    broken "cannot resolve $name from the environment or $file (got '$value')"
  fi
  printf '%s' "$value"
}

cd "$checkout"

splice_version="$(setting SPLICE_VERSION compose/.env.defaults)"
postgres_version="$(setting POSTGRES_VERSION compose/modules/localnet/compose.env)"
protocol_version="${CANTON_PROTOCOL_VERSION:-default}"
current_key="splice=${splice_version} postgres=${postgres_version} protocol=${protocol_version}"

previous_key="<none>"
if [ -f "$key_file" ]; then
  previous_key="$(cat "$key_file")"
fi

log "state key previous=${previous_key}"
log "state key current=${current_key}"

# The scheduled Terraform `down` apply (server_enabled=false destroys the
# server resource) on a prior lifecycle event abruptly killed dockerd.
# Docker's persisted engine state is then left inconsistent in several ways:
# stale containers with nil RW layers, a corrupt BuildKit cache, and
# locally-built images referencing missing content-store blobs. Reset that
# mutable engine state via `make down` / `make clean` below so `make up`
# recreates containers and rebuilds images from the (intact, crash-consistent)
# pulled base layers. There is no upstream issue reference; the
# inconsistent-engine-state was observed empirically.
if [ "$previous_key" = "$current_key" ]; then
  log "decision: keeping volumes (state key unchanged) — make down"
  make down || echo "WARN: make down failed (likely nothing to tear down) — continuing" >&2
else
  log "decision: wiping volumes (state key changed or absent) — make clean"
  make clean || failed "make clean failed — refusing to bring LocalNet up on volumes written by another version"
  log "volumes wiped"
fi

docker buildx prune -af || echo "WARN: buildx prune failed — make up may reuse stale cache" >&2
mapfile -t built_images < <(docker images --format '{{.Repository}}:{{.Tag}}' | grep '^localnet-' || true)
if [ "${#built_images[@]}" -gt 0 ]; then
  docker rmi -f "${built_images[@]}" || echo "WARN: failed to remove ${built_images[*]} — make up may reuse corrupt images" >&2
fi

"$@" || failed "bring-up failed ($*) — state key not recorded, so the next boot wipes again"

mkdir -p "$(dirname "$key_file")"
printf '%s\n' "$current_key" >"${key_file}.tmp"
mv "${key_file}.tmp" "$key_file"
log "recorded state key ${current_key} in ${key_file}"
