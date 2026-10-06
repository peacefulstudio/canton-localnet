#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Resolves the canton-localnet binary the rest of the action uses, per
# input 'cli':
#   source (default) — go build it from $ACTION_PATH/cli (measured ~9s
#                       with a warm setup-go cache).
#   release           — download the release asset matching $ACTION_REF
#                       and verify it against that release's checksums.txt.
#                       Fails closed: valid only when $ACTION_REF is a
#                       literal release tag, and any download or checksum
#                       problem aborts rather than falling back to source.
#
# Required env: CLI_MODE, ACTION_PATH, ACTION_REF, RUNNER_TEMP, GITHUB_OUTPUT,
# GITHUB_PATH.

set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$here/lib.sh"

bin_dir="$RUNNER_TEMP/canton-localnet/bin"
mkdir -p "$bin_dir"
: >"$(canton_localnet_timings_file)"
resolve_started="$(date +%s)"
cli_path="$bin_dir/canton-localnet"

build_from_source() {
  echo "Building canton-localnet from source (${ACTION_PATH}/cli)"
  (cd "$ACTION_PATH/cli" && go build -o "$cli_path" ./cmd/canton-localnet)
}

download_release() {
  local release_repo="https://github.com/peacefulstudio/canton-localnet"
  # Matches release.yml's own tag grammar exactly (its "Parse tag" step):
  # v<splice>-<patch> optionally followed by a .<label> such as
  # v0.8.3-1.preview.1. A narrower v<splice>-<patch>-only pattern would
  # reject valid, published preview releases.
  if [[ ! "$ACTION_REF" =~ ^v[0-9]+\.[0-9]+\.[0-9]+-[1-9][0-9]*(\.[0-9A-Za-z][0-9A-Za-z.]*)?$ ]]; then
    canton_localnet_log_error "canton-localnet action: cli: release requires a tag-pinned 'uses:' (e.g. uses: peacefulstudio/canton-localnet@v<ver>); got action_ref '${ACTION_REF}', which is not a release tag — a SHA-pinned 'uses:' resolves action_ref to the SHA, not the tag it points at. Use cli: source with SHA pins, or switch this 'uses:' to a literal tag if you need cli: release."
    return 1
  fi
  local arch arch_label
  arch="$(uname -m)"
  case "$arch" in
    x86_64)  arch_label="amd64" ;;
    aarch64) arch_label="arm64" ;;
    *)
      canton_localnet_log_error "canton-localnet action: cli: release only ships linux amd64 and linux arm64; runner reports arch '${arch}'. Use cli: source."
      return 1
      ;;
  esac
  local version="${ACTION_REF#v}"
  local asset="canton-localnet-${version}-linux-${arch_label}"
  local base_url="${release_repo}/releases/download/${ACTION_REF}"
  local work_dir="$RUNNER_TEMP/canton-localnet/release-download"
  mkdir -p "$work_dir"
  echo "Downloading ${asset} and checksums.txt from ${base_url}"
  if ! curl -fsSL --retry 2 -o "$work_dir/$asset" "$base_url/$asset"; then
    canton_localnet_log_error "canton-localnet action: cli: release could not download ${asset} from ${base_url} — the release may not carry that asset yet. Use cli: source."
    return 1
  fi
  if ! curl -fsSL --retry 2 -o "$work_dir/checksums.txt" "$base_url/checksums.txt"; then
    canton_localnet_log_error "canton-localnet action: cli: release could not download checksums.txt from ${base_url}. Use cli: source."
    return 1
  fi
  # release.yml generates checksums.txt with `sha256sum ./*`, so each
  # recorded filename is "./<asset>", not a bare "<asset>".
  if ! (cd "$work_dir" && grep -E "[[:space:]]\./${asset}\$" checksums.txt | sha256sum -c -); then
    canton_localnet_log_error "canton-localnet action: cli: release checksum verification failed for ${asset} — refusing to run an unverified binary."
    return 1
  fi
  install -m 0755 "$work_dir/$asset" "$cli_path"
}

case "$CLI_MODE" in
  source)
    build_from_source
    ;;
  release)
    download_release
    ;;
  *)
    canton_localnet_log_error "canton-localnet action: unknown 'cli' input '${CLI_MODE}' (want source or release)"
    exit 1
    ;;
esac

"$cli_path" --version >/dev/null
canton_localnet_record_phase "CLI resolve ($CLI_MODE)" "$resolve_started"

echo "$bin_dir" >>"$GITHUB_PATH"
echo "cli-path=$cli_path" >>"$GITHUB_OUTPUT"
