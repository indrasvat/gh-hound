#!/usr/bin/env bash
# Cross-compile gh-hound for gh-extension-precompile.
set -euo pipefail

tag="${1:-${VERSION:-${GITHUB_REF_NAME:-dev}}}"
commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
date="${DATE:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
ext_name="gh-hound"
cmd="./cmd/gh-hound"
ldflags="-s -w -X main.version=${tag} -X main.commit=${commit} -X main.date=${date}"

platforms=(
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

rm -rf dist
mkdir -p dist

for platform in "${platforms[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  suffix=""
  if [ "$goos" = "windows" ]; then
    suffix=".exe"
  fi
  out="dist/${ext_name}_${tag}_${goos}-${goarch}${suffix}"
  echo "building ${goos}/${goarch} -> ${out}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="${ldflags}" -o "$out" "$cmd"
done

(
  cd dist
  shasum -a 256 gh-hound_* >checksums.txt
)

ls -lh dist
