#!/usr/bin/env bash
set -euo pipefail

output_dir="${1:-dist/chitchat-release}"
target_os="${GOOS:-$(go env GOOS)}"
target_arch="${GOARCH:-$(go env GOARCH)}"
go_version="$(go env GOVERSION)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

build() {
  local output="$1"
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
    go build -trimpath -buildvcs=false -ldflags="-s -w" \
    -o "$output" ./cmd/chitchat
}

build "$temp_dir/chitchat-1"
build "$temp_dir/chitchat-2"

if ! cmp -s "$temp_dir/chitchat-1" "$temp_dir/chitchat-2"; then
  echo "reproducible build check failed: build outputs differ" >&2
  exit 1
fi

mkdir -p "$output_dir"
artifact="$output_dir/chitchat-$target_os-$target_arch"
checksum="$artifact.sha256"
cp "$temp_dir/chitchat-1" "$artifact"
(
  cd "$output_dir"
  sha256sum "$(basename "$artifact")" > "$(basename "$checksum")"
)

printf 'artifact=%s\n' "$artifact"
printf 'sha256=%s\n' "$(awk '{print $1}' "$checksum")"
printf 'go=%s target=%s/%s cgo=0\n' "$go_version" "$target_os" "$target_arch"
