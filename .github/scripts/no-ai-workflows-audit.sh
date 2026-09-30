#!/usr/bin/env bash
# Copyright (c) 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 [ROOT]

Audit ROOT/.github for agentic workflows and references that must not reach
the public twin, and exit non-zero on any hit.

When ROOT/.gitpublic and ROOT/scripts/gitpublic-paths.sh both exist, only the
.github paths .gitpublic allowlists for promotion are checked — the internal
repo's own .github/workflows/ deliberately keeps its agentic workflows
untracked for the public side. Otherwise the actual ROOT/.github tree is
checked directly, which is the shape the public twin's own checkout takes
since neither file is itself promoted there.

ROOT defaults to the current directory.
EOF
}

FORCE_WHOLE_TREE=false

while [[ "${1:-}" == --* ]]; do
  case "$1" in
    --whole-tree) FORCE_WHOLE_TREE=true; shift ;;
    -h | --help) usage; exit 0 ;;
    *) echo "no-ai-workflows-audit: unknown flag: $1" >&2; exit 2 ;;
  esac
done

ROOT="${1:-.}"

resolve_scan_targets() {
  SCAN_TARGETS=()
  if ! $FORCE_WHOLE_TREE && [ -f "$ROOT/.gitpublic" ] && [ -f "$ROOT/scripts/gitpublic-paths.sh" ]; then
    local resolved path
    if ! resolved="$(bash "$ROOT/scripts/gitpublic-paths.sh" "$ROOT")"; then
      echo "no-ai-workflows-audit: scripts/gitpublic-paths.sh rejected $ROOT/.gitpublic — refusing to scan" >&2
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

BANNED_WORKFLOW_FILENAMES=(claude.yaml claude.yml ci-autofix.yaml ci-autofix.yml automerge.yaml automerge.yml merge-conflict-autofix.yaml merge-conflict-autofix.yml peaceful-bot-review.yaml peaceful-bot-review.yml hetzner-localnet-schedule.yaml hetzner-localnet-schedule.yml)
OWN_PATTERN_DEFINITION_EXCLUDES=(':!.github/scripts/leak-check.sh' ':!.github/scripts/no-ai-workflows-audit.sh')

HITS=0
ACTION_SCAN_TARGETS=()

scanning_whole_tree() {
  [ "${#SCAN_TARGETS[@]}" -eq 1 ] && [ "${SCAN_TARGETS[0]}" = "." ]
}

flag_banned_filename_present() {
  local workflows_dir="$ROOT/.github/workflows" banned
  for banned in "${BANNED_WORKFLOW_FILENAMES[@]}"; do
    if [ -e "$workflows_dir/$banned" ]; then
      echo "no-AI-workflows violation: .github/workflows/$banned present on public repo"
      HITS=1
    fi
  done
}

flag_banned_filename_allowlisted() {
  local banned target clean
  for banned in "${BANNED_WORKFLOW_FILENAMES[@]}"; do
    for target in "${SCAN_TARGETS[@]}"; do
      clean="${target%/}"
      if [ "$clean" = ".github/workflows/$banned" ]; then
        echo "no-AI-workflows violation: .github/workflows/$banned is allowlisted in .gitpublic for promotion"
        HITS=1
      elif [ "$clean" = ".github/workflows" ]; then
        echo "no-AI-workflows violation: .github/workflows/$banned is covered by the directory entry '$target' allowlisted in .gitpublic for promotion"
        HITS=1
      fi
    done
  done
}

if scanning_whole_tree; then
  ACTION_SCAN_TARGETS+=(".")
  flag_banned_filename_present
else
  ACTION_SCAN_TARGETS+=("${SCAN_TARGETS[@]}")
  flag_banned_filename_allowlisted
fi

if [ "${#ACTION_SCAN_TARGETS[@]}" -gt 0 ]; then
  found=0
  matches=$(git -C "$ROOT" grep -nIiE \
    '["'"'"']?uses["'"'"']?\s*:.*((anthropics?/(claude|codex)|openai/codex|github/copilot)|/(claude|ci-autofix|merge-conflict-autofix|automerge|peaceful-bot-review|hetzner-localnet-schedule)\.ya?ml)' \
    -- "${ACTION_SCAN_TARGETS[@]}" "${OWN_PATTERN_DEFINITION_EXCLUDES[@]}") || found=$?
  if [ "$found" -gt 1 ]; then
    echo "no-ai-workflows-audit: git grep failed (status $found) for [AI-tooling action references]" >&2
    exit 2
  fi
  if [ -n "$matches" ]; then
    printf 'no-AI-workflows violation: AI-tooling action or private-only reusable workflow referenced:\n%s\n' "$matches"
    HITS=1
  fi

  found=0
  internal_refs=$(git -C "$ROOT" grep -nIE 'peacefulstudio/[a-z0-9-]+-internal' -- "${ACTION_SCAN_TARGETS[@]}" "${OWN_PATTERN_DEFINITION_EXCLUDES[@]}") || found=$?
  if [ "$found" -gt 1 ]; then
    echo "no-ai-workflows-audit: git grep failed (status $found) for [-internal repository references]" >&2
    exit 2
  fi
  if [ -n "$internal_refs" ]; then
    printf 'no-AI-workflows violation: -internal repository referenced:\n%s\n' "$internal_refs"
    HITS=1
  fi

  found=0
  direct_ai=$(git -C "$ROOT" grep -nIiE \
    'api\.(anthropic\.com|openai\.com)|(ANTHROPIC|OPENAI)_API_KEY|@openai/codex|@anthropic-ai/claude-code|(^|[^[:alnum:]_/-])(codex +exec|claude +(-p|--print)|gh +copilot)([^[:alnum:]_-]|$)' \
    -- "${ACTION_SCAN_TARGETS[@]}" "${OWN_PATTERN_DEFINITION_EXCLUDES[@]}") || found=$?
  if [ "$found" -gt 1 ]; then
    echo "no-ai-workflows-audit: git grep failed (status $found) for [direct AI API or CLI invocations]" >&2
    exit 2
  fi
  if [ -n "$direct_ai" ]; then
    printf 'no-AI-workflows violation: direct AI API/CLI invocation or credential:\n%s\n' "$direct_ai"
    HITS=1
  fi

  ml_hits=""
  while IFS= read -r rel; do
    f="$ROOT/$rel"
    [ -f "$f" ] || continue
    hit=$(perl -0777 -ne '
      while (/["'"'"']?uses["'"'"']?\s*:\s*[>|][-+]?\n[ \t]+(\S[^\n]*)/mg) {
        my $v = $1; $v =~ s/\s+$//;
        if ($v =~ m{anthropics?/(claude|codex)|openai/codex|github/copilot|/(claude|ci-autofix|merge-conflict-autofix|automerge|peaceful-bot-review|hetzner-localnet-schedule)\.ya?ml}) {
          print "$ARGV: uses (block scalar): $v\n";
        }
      }
    ' "$f" 2>/dev/null) || true
    [ -n "$hit" ] && ml_hits+="$hit"$'\n'
  done < <(git -C "$ROOT" ls-files -- "${ACTION_SCAN_TARGETS[@]}" 2>/dev/null | grep -iE '\.(ya?ml)$' || true)
  if [ -n "$ml_hits" ]; then
    printf 'no-AI-workflows violation: AI-tooling action in YAML block scalar:\n%s\n' "$ml_hits"
    HITS=1
  fi
fi

if [ "$HITS" -ne 0 ]; then
  exit 1
fi
echo "no-ai-workflows-audit: clean"
