#!/bin/bash
# Cross-compile IconCraft for Linux and Windows. CGO stays off so a
# macOS or Linux machine can produce every binary. Signing is not done.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p build
for spec in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${spec%/*}"
  arch="${spec#*/}"
  ext=""
  if [ "$os" = "windows" ]; then
    ext=".exe"
  fi
  out="build/iconcraft-${os}-${arch}${ext}"
  echo "building ${out}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -o "$out" ./cmd/iconcraft
done
echo "built:"
ls -l build/iconcraft-linux-* build/iconcraft-windows-*
