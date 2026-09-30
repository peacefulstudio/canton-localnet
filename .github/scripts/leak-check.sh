#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 [ROOT]

Scan ROOT for content that must not reach the public twin, and exit non-zero
on any hit.

When ROOT/.gitpublic and ROOT/scripts/gitpublic-paths.sh both exist, the scan
is restricted to exactly the paths .gitpublic allowlists for promotion — the
internal repo carries internal-only content outside that allowlist by design.
Otherwise the whole of ROOT is scanned, which is the shape the public twin's
own checkout takes since neither file is itself promoted there. A rejected
.gitpublic (scripts/gitpublic-paths.sh exiting non-zero) is a hard error, not
an empty scan.

The terraform state/vars/keys and .env/.env.local checks are tracked-file
existence checks, not content scans, and always cover the whole of ROOT: a
force-added secret-shaped file is a violation regardless of whether its path
is promotion-eligible.

ROOT defaults to the current directory.
EOF
}

FORCE_WHOLE_TREE=false

while [[ "${1:-}" == --* ]]; do
  case "$1" in
    --whole-tree) FORCE_WHOLE_TREE=true; shift ;;
    -h | --help) usage; exit 0 ;;
    *) echo "leak-check: unknown flag: $1" >&2; exit 2 ;;
  esac
done

ROOT="${1:-.}"

resolve_scan_targets() {
  SCAN_TARGETS=()
  if ! $FORCE_WHOLE_TREE && [ -f "$ROOT/.gitpublic" ] && [ -f "$ROOT/scripts/gitpublic-paths.sh" ]; then
    local resolved path
    if ! resolved="$(bash "$ROOT/scripts/gitpublic-paths.sh" "$ROOT")"; then
      echo "leak-check: scripts/gitpublic-paths.sh rejected $ROOT/.gitpublic — refusing to scan" >&2
      exit 2
    fi
    while IFS= read -r path; do
      [ -n "$path" ] || continue
      [ -e "$ROOT/${path%/}" ] || continue
      SCAN_TARGETS+=("${path%/}")
    done <<<"$resolved"
  else
    SCAN_TARGETS+=(".")
  fi
}

resolve_scan_targets

if [ "${#SCAN_TARGETS[@]}" -eq 0 ]; then
  echo "leak-check: .gitpublic lists no paths that exist in $ROOT — nothing to scan"
  exit 0
fi

BINARY_FIXTURE_EXCLUDES=(':!*.dar' ':!*.binpb' ':!*.png')
OWN_SCRIPT_EXCLUDES=(':!.github/scripts/leak-check.sh' ':!.github/scripts/no-ai-workflows-audit.sh')
CONTENT_SCAN_PATHSPECS=("${SCAN_TARGETS[@]}" "${BINARY_FIXTURE_EXCLUDES[@]}" "${OWN_SCRIPT_EXCLUDES[@]}")
CRED_SCAN_PATHSPECS=("${SCAN_TARGETS[@]}" "${BINARY_FIXTURE_EXCLUDES[@]}")

HITS=0

scan_content() {
  local label="$1" pattern="$2"
  local matches found=0
  matches=$(git -C "$ROOT" grep -nIiP -e "$pattern" -- "${CONTENT_SCAN_PATHSPECS[@]}") || found=$?
  if [ "$found" -gt 1 ]; then
    echo "leak-check: git grep failed (status $found) for [$label]" >&2
    exit 2
  fi
  if [ -n "$matches" ]; then
    printf 'LEAK [%s]:\n%s\n\n' "$label" "$matches"
    HITS=1
  fi
}

scan_credential() {
  local label="$1" pattern="$2"
  local matches found=0 locations
  matches=$(git -C "$ROOT" grep -nIiP -e "$pattern" -- "${CRED_SCAN_PATHSPECS[@]}") || found=$?
  if [ "$found" -gt 1 ]; then
    echo "leak-check: git grep failed (status $found) for [$label]" >&2
    exit 2
  fi
  if [ -n "$matches" ]; then
    locations=$(printf '%s\n' "$matches" | cut -d: -f1,2)
    printf 'LEAK [%s]:\n%s\n\n' "$label" "$locations"
    HITS=1
  fi
}

scan_tracked_files() {
  local label="$1"; shift
  local files
  files=$(git -C "$ROOT" ls-files -- "$@")
  if [ -n "$files" ]; then
    printf 'LEAK [%s]:\n%s\n\n' "$label" "$files"
    HITS=1
  fi
}

scan_instruction_file_paths() {
  local files hit found=0
  files=$(git -C "$ROOT" ls-files -- "${SCAN_TARGETS[@]}")
  [ -n "$files" ] || return 0
  hit=$(printf '%s\n' "$files" | grep -iE '(^|/)(AGENTS|CLAUDE)\.md$|(^|/)copilot-instructions\.md$') || found=$?
  if [ "$found" -gt 1 ]; then
    echo "leak-check: grep failed (status $found) for [instruction-file paths]" >&2
    exit 2
  fi
  if [ -n "$hit" ]; then
    printf 'LEAK [instruction file reachable by path under a curated directory]:\n%s\n\n' "$hit"
    HITS=1
  fi
}

INTERNAL_REPO_PATTERN='peacefulstudio/[a-z0-9-]+-internal'
PEACEFUL_INTERNAL_REPO_NAMES_FILE="$ROOT/scripts/peaceful-internal-repo-names.txt"
if [ -f "$PEACEFUL_INTERNAL_REPO_NAMES_FILE" ]; then
  PEACEFUL_INTERNAL_REPO_NAMES=()
  while IFS= read -r name || [ -n "$name" ]; do
    [ -n "$name" ] || continue
    PEACEFUL_INTERNAL_REPO_NAMES+=("$name")
  done <"$PEACEFUL_INTERNAL_REPO_NAMES_FILE"
  if [ "${#PEACEFUL_INTERNAL_REPO_NAMES[@]}" -gt 0 ]; then
    INTERNAL_REPO_PATTERN+="|\\b($(IFS='|'; echo "${PEACEFUL_INTERNAL_REPO_NAMES[*]}"))-internal\\b"
  fi
fi

scan_content "references to private -internal repos (peacefulstudio/-prefixed or bare)" \
  "$INTERNAL_REPO_PATTERN"

scan_content "agent instruction files" \
  'CLAUDE\.md|AGENTS\.md|\.github/copilot-instructions'

scan_instruction_file_paths

scan_internal_doc_paths() {
  local files hit found=0
  files=$(git -C "$ROOT" ls-files -- "${SCAN_TARGETS[@]}")
  [ -n "$files" ] || return 0
  hit=$(printf '%s\n' "$files" | grep -E '(^|/)docs/internal/') || found=$?
  if [ "$found" -gt 1 ]; then
    echo "leak-check: grep failed (status $found) for [internal docs paths]" >&2
    exit 2
  fi
  if [ -n "$hit" ]; then
    printf 'LEAK [internal docs reachable by path]:\n%s\n\n' "$hit"
    HITS=1
  fi
}

scan_content "internal docs paths" \
  'docs/internal/'
scan_internal_doc_paths

scan_content "non-public peaceful.studio addresses" \
  '\b(?!(?:security|conduct)@)[a-z0-9._-]+@peaceful\.studio'

scan_content "ASCII-mangled legal entity name (OU instead of OÜ)" \
  'Peaceful Studio OU\b'

scan_credential "credential fragments" \
  'sk-ant-[a-z0-9]|A[KS]IA[A-Z0-9]{16}|-{5}BEGIN[A-Z ]*PRIVATE KEY'

scan_content "AI signatures in files" \
  'co-authored-by: claude|generated with.*claude|@ai-generated|claude\.ai/code|noreply@anthropic'

scan_credential "literal cloud credential assigned (OVH/Hetzner)" \
  "\b(TF_VAR_hcloud_token|TF_VAR_HCLOUD_TOKEN|hcloud_token|HCLOUD_TOKEN|OVH_(APPLICATION|CONSUMER)_(KEY|SECRET))\b[\"']?\s*[=:]\s*[\"']?[A-Za-z0-9]{8,}"

scan_credential "AWS temporary credential assigned" \
  "\b(AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN)\b[\"']?\s*[=:]\s*[\"']?[A-Za-z0-9/+=]{8,}"

scan_tracked_files "committed terraform state/vars/keys" \
  ':(glob)terraform/**/backend.hcl' \
  ':(glob)terraform/**/*.tfvars' \
  ':(glob)terraform/**/*.tfvars.json' \
  ':(glob)terraform/**/*.pem' \
  ':(glob)**/*.tfstate' \
  ':(glob)**/*.tfstate.*'

scan_tracked_files "committed .env/.env.local (real secret overrides)" \
  ':(glob)**/.env' \
  ':(glob)**/.env.local'

if [ "$HITS" -ne 0 ]; then
  exit 1
fi
echo "leak-check: clean"
