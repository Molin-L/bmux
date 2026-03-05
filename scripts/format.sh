#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "Formatting Go files..."
mapfile -d '' go_files < <(
  find . \
    -type d \( -name .git -o -name .worktrees -o -name .bmux -o -name bin \) -prune -o \
    -type f -name '*.go' -print0
)

if (( ${#go_files[@]} > 0 )); then
  gofmt -w "${go_files[@]}"
else
  echo "No Go files found."
fi

if command -v shfmt >/dev/null 2>&1; then
  echo "Formatting shell scripts..."
  mapfile -d '' sh_files < <(
    find . \
      -type d \( -name .git -o -name .worktrees -o -name .bmux -o -name bin \) -prune -o \
      -type f -name '*.sh' -print0
  )

  if (( ${#sh_files[@]} > 0 )); then
    shfmt -w "${sh_files[@]}"
  else
    echo "No shell scripts found."
  fi
else
  echo "shfmt not found; skipping shell script formatting."
fi

echo "Formatting complete."
