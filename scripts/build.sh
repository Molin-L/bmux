#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: ./scripts/build.sh [--release|--debug]

Options:
  --release   Build optimized binary (default)
  --debug     Build debug binary for testing (no optimizations)
  -h, --help  Show this help message
USAGE
}

mode="release"

if [[ $# -gt 1 ]]; then
  usage
  exit 1
fi

if [[ $# -eq 1 ]]; then
  case "$1" in
    --release)
      mode="release"
      ;;
    --debug)
      mode="debug"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
fi

mkdir -p bin

if [[ "$mode" == "release" ]]; then
  echo "Building release binary..."
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/bmux ./cmd/bmux
  echo "Built: ./bin/bmux (release)"
else
  echo "Building debug binary..."
  CGO_ENABLED=0 go build -gcflags="all=-N -l" -o ./bin/bmux-debug ./cmd/bmux
  echo "Built: ./bin/bmux-debug (debug)"
fi
