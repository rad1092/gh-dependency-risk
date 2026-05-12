#!/usr/bin/env bash
set -euo pipefail

tag="${1:-dev}"
version="${tag}"
commit="${BUILD_COMMIT:-none}"
date="${BUILD_DATE:-unknown}"

platforms=(
  darwin-amd64
  darwin-arm64
  freebsd-386
  freebsd-amd64
  freebsd-arm64
  linux-386
  linux-amd64
  linux-arm
  linux-arm64
  windows-386
  windows-amd64
  windows-arm64
)

mkdir -p dist

mapfile -t supported_platforms < <(go tool dist list)

for platform in "${platforms[@]}"; do
  goos="${platform%-*}"
  goarch="${platform#*-}"
  if [[ " ${supported_platforms[*]} " != *" ${goos}/${goarch} "* ]]; then
    echo "warning: skipping unsupported platform ${platform}" >&2
    continue
  fi

  ext=""
  if [[ "${goos}" == "windows" ]]; then
    ext=".exe"
  fi

  GOOS="${goos}" \
  GOARCH="${goarch}" \
  CGO_ENABLED=0 \
  go build \
    -trimpath \
    -ldflags="-s -w -X github.com/rad1092/gh-dependency-risk/cmd.version=${version} -X github.com/rad1092/gh-dependency-risk/cmd.commit=${commit} -X github.com/rad1092/gh-dependency-risk/cmd.date=${date}" \
    -o "dist/${platform}${ext}" \
    .
done
