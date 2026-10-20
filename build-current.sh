#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p build
rm -f build/BUILD-COMPLETE
export GOFLAGS="-p=4"
gofmt -w cmd internal
node --check internal/native/bridge.js
node --check internal/native/window.js
node --check internal/server/ui/app.js
node --test tests/*.test.cjs > build/bridge-regression.tap
go test -timeout=60s -count=1 -json ./... > build/tests-current.jsonl
go test -race -timeout=60s -count=1 ./... > build/race-current.txt
go vet ./... > build/vet-current.txt 2>&1
python3 scripts/build_macos.py > build/build-current.txt 2>&1
for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go test -c -o "build/native-tests-$arch" ./internal/native
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go test -c -o "build/core-tests-$arch" ./internal/core
done
go test -run='^$' -bench=BenchmarkWarmInventoryScan -benchtime=1s ./internal/core > build/benchmark-current.txt
printf 'OK\n' > build/BUILD-COMPLETE
