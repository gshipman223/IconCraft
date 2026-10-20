#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p build
rm -f build/BUILD-COMPLETE
export GOFLAGS="-p=4"
# Stage markers: test output goes to files (kept as artifacts), so without
# these a CI failure is a bare exit code with no indication of the stage.
stage() { printf '\n== %s ==\n' "$1"; }
stage "format + JS checks"
gofmt -w cmd internal
node --check internal/native/bridge.js
node --check internal/native/window.js
node --check internal/server/ui/app.js
node --test tests/*.test.cjs > build/bridge-regression.tap
stage "go test"
go test -timeout=180s -count=1 -json ./... > build/tests-current.jsonl || { grep -E '"Action":"fail"' build/tests-current.jsonl | tail -5; exit 1; }
stage "go test -race"
go test -race -timeout=180s -count=1 ./... > build/race-current.txt || { tail -20 build/race-current.txt; exit 1; }
stage "go vet"
go vet ./... > build/vet-current.txt 2>&1
stage "build macOS app"
python3 scripts/build_macos.py > build/build-current.txt 2>&1 || { tail -20 build/build-current.txt; exit 1; }
stage "cross-arch test compiles"
for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go test -c -o "build/native-tests-$arch" ./internal/native
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go test -c -o "build/core-tests-$arch" ./internal/core
done
stage "benchmark"
go test -run='^$' -bench=BenchmarkWarmInventoryScan -benchtime=1s ./internal/core > build/benchmark-current.txt
printf 'OK\n' > build/BUILD-COMPLETE
stage "done"
