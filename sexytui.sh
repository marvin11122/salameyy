#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$root_dir"
export CGO_ENABLED=0

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
binary="$tmp_dir/salamey"

go build -mod=readonly -trimpath -o "$binary" .

if [[ "${1:-}" == "--model" ]]; then
  if [[ $# -lt 2 ]]; then
    printf '%s\n' 'Usage: ./sexytui.sh [--model /path/to/model.gguf]' >&2
    exit 2
  fi
  model="$2"
  shift 2
  if ! "$binary" llama start --model "$model" --select; then
    status="$("$binary" llama status 2>/dev/null || true)"
    if [[ ! "$status" =~ reachable[[:space:]]+true ]]; then
      printf '%s\n' 'Could not start llama.cpp.' >&2
      exit 1
    fi
    printf '%s\n' 'Using the already-running llama.cpp runtime.' >&2
  fi
fi

"$binary" "$@"
