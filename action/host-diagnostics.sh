#!/usr/bin/env bash
# Copyright 2026 Peaceful Studio OÜ
# SPDX-License-Identifier: Apache-2.0
#
# Host-level evidence for a stalled or failed LocalNet boot.
#
#   host-diagnostics.sh start-sampler <samples-file>
#   host-diagnostics.sh stop-sampler <samples-file>
#   host-diagnostics.sh capture <out-dir> [samples-file]
#
# Every command is best-effort and bounded: it must never fail the step that
# calls it, and never read env files or compose config.
#
# Container state, stats and logs cover only this LocalNet's compose project
# (COMPOSE_PROJECT_NAME, else `localnet`, the name compose derives from
# compose/modules/localnet): a self-hosted runner's daemon is shared with
# other jobs. An invalid project name skips them rather than widening the scan.

set -uo pipefail

sample_interval_seconds=10
max_samples=720
command_timeout_seconds=30
log_tail_lines=200
log_container_pattern='splice|canton'
default_compose_project=localnet
compose_project_name_pattern='^[a-z0-9][a-z0-9_-]*$'
container_health_format='{{.Name}} state={{.State.Status}} exit={{.State.ExitCode}} oomkilled={{.State.OOMKilled}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}'

run_into() {
  local file="$1"
  shift
  timeout "$command_timeout_seconds" "$@" >"$file" 2>&1 || true
}

kernel_oom_lines() {
  { dmesg 2>/dev/null || sudo -n dmesg 2>/dev/null; } | grep -i -E 'oom|killed process'
}

compose_project() {
  local project="${COMPOSE_PROJECT_NAME:-$default_compose_project}"
  [[ "$project" =~ $compose_project_name_pattern ]] || return 1
  echo "$project"
}

capture_project_containers() {
  local out_dir="$1" project project_filter name
  local -a names
  if ! project="$(compose_project)"; then
    echo "skipped: COMPOSE_PROJECT_NAME is not a valid compose project name" >"$out_dir/container-scope.txt"
    return 0
  fi
  project_filter="label=com.docker.compose.project=$project"
  echo "compose project: $project" >"$out_dir/container-scope.txt"
  run_into "$out_dir/docker-ps.txt" docker ps -a --filter "$project_filter"
  mapfile -t names < <(timeout "$command_timeout_seconds" docker ps -a --filter "$project_filter" --format '{{.Names}}' 2>/dev/null)
  [ "${#names[@]}" -gt 0 ] || return 0
  run_into "$out_dir/docker-stats.txt" docker stats --no-stream "${names[@]}"
  run_into "$out_dir/container-health.txt" docker inspect --format "$container_health_format" "${names[@]}"
  for name in "${names[@]}"; do
    [[ "$name" =~ $log_container_pattern ]] || continue
    run_into "$out_dir/logs-${name}.txt" docker logs --tail "$log_tail_lines" "$name"
  done
}

sample_memory() {
  local sample_file="$1" total_kib available_kib
  [ -r /proc/meminfo ] || return 0
  local taken=0
  while [ "$taken" -lt "$max_samples" ]; do
    taken=$((taken + 1))
    total_kib="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
    available_kib="$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)"
    echo "$(date +%s) $(((total_kib - available_kib) / 1024)) $((available_kib / 1024))" >>"$sample_file"
    sleep "$sample_interval_seconds"
  done
}

start_sampler() {
  local sample_file="$1"
  mkdir -p "$(dirname "$sample_file")"
  : >"$sample_file"
  setsid nohup bash "${BASH_SOURCE[0]}" sample "$sample_file" >/dev/null 2>&1 &
  echo $! >"$sample_file.pid"
}

stop_sampler() {
  local pid_file="$1.pid"
  [ -f "$pid_file" ] || return 0
  kill "$(cat "$pid_file")" 2>/dev/null || true
  rm -f "$pid_file"
}

describe_peak() {
  local sample_file="$1"
  if [ ! -s "$sample_file" ]; then
    echo "no memory samples recorded"
    return 0
  fi
  sort -k2 -n "$sample_file" | tail -1 | awk '{printf "peak used %d MiB (available %d MiB) at epoch %d\n", $2, $3, $1}'
  awk 'NR == 1 || $3 < min {min = $3} END {printf "lowest available %d MiB over %d samples\n", min, NR}' "$sample_file"
}

capture() {
  local out_dir="$1" sample_file="${2:-}"
  mkdir -p "$out_dir"

  run_into "$out_dir/free-m.txt" free -m
  run_into "$out_dir/nproc.txt" nproc
  run_into "$out_dir/dmesg-oom.txt" bash -c "$(declare -f kernel_oom_lines); kernel_oom_lines"
  capture_project_containers "$out_dir"
  if [ -n "$sample_file" ]; then
    describe_peak "$sample_file" >"$out_dir/memory-peak.txt" 2>&1 || true
    cp "$sample_file" "$out_dir/memory-samples.txt" 2>/dev/null || true
  fi

  echo "::group::Host diagnostics"
  for file in free-m nproc dmesg-oom container-scope docker-stats container-health memory-peak; do
    [ -f "$out_dir/$file.txt" ] || continue
    echo "--- $file"
    cat "$out_dir/$file.txt"
  done
  echo "::endgroup::"
}

case "${1:-}" in
  start-sampler) start_sampler "${2:?samples file required}" ;;
  stop-sampler) stop_sampler "${2:?samples file required}" ;;
  sample) sample_memory "${2:?samples file required}" ;;
  capture) capture "${2:?out dir required}" "${3:-}" ;;
  *)
    echo "usage: host-diagnostics.sh start-sampler <samples-file> | stop-sampler <samples-file> | capture <out-dir> [samples-file]" >&2
    exit 2
    ;;
esac
