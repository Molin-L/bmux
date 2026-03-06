#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: ./scripts/docker-build.sh --platform <linux/amd64|darwin/arm64>

Options:
  --platform  Target platform (required): linux/amd64 or darwin/arm64
  -h, --help  Show this help message
USAGE
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

platform=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform)
      if [[ $# -lt 2 ]]; then
        echo "Error: --platform requires a value." >&2
        usage
        exit 1
      fi
      platform="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$platform" ]]; then
  echo "Error: --platform is required." >&2
  usage
  exit 1
fi

case "$platform" in
  linux/amd64)
    goos="linux"
    goarch="amd64"
    ;;
  darwin/arm64)
    goos="darwin"
    goarch="arm64"
    ;;
  *)
    echo "Error: Unsupported platform '$platform'. Supported: linux/amd64, darwin/arm64." >&2
    exit 1
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "Error: docker is not installed or not in PATH." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Error: docker daemon is not reachable. Start Docker and try again." >&2
  exit 1
fi

out_dir="./release/${goos}/${goarch}"
out_file="${out_dir}/bmux"
mkdir -p "$out_dir"

echo "Building for ${platform} using Docker..."
docker run --rm \
  -v "${ROOT_DIR}:/src" \
  -w /src \
  -e "CGO_ENABLED=0" \
  -e "GOOS=${goos}" \
  -e "GOARCH=${goarch}" \
  golang:1.26 \
  go build -trimpath -ldflags="-s -w" -o "${out_file}" ./cmd/bmux

echo "Built: ${out_file}"
