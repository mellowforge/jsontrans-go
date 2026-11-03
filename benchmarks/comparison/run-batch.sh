#!/bin/sh
# Run from this directory after production changes are frozen and other CPU work
# has stopped. The old source needs only the prepared API, never the batch API.
set -eu

snapshot=${SOURCE_SNAPSHOT:-/private/tmp/jsontrans-before-batch}
results=${RESULTS_DIR:-testdata/results/batch}
duration=${BENCHTIME:-200ms}
samples=${SAMPLES:-5}
mkdir -p "$results"
scratch=$(mktemp -d /private/tmp/jsontrans-batch-bench.XXXXXX)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
export GOTOOLCHAIN=local
export GOCACHE=${GOCACHE:-/private/tmp/jsontrans-batch-benchmark-gocache}

go version > "$results/environment.txt"
go env GOOS GOARCH GOMAXPROCS >> "$results/environment.txt"
printf 'benchtime=%s\nsamples=%s\nsnapshot=%s\n' "$duration" "$samples" "$snapshot" >> "$results/environment.txt"
cp go.mod "$scratch/before.mod"
cp go.sum "$scratch/before.sum"
go mod edit -modfile="$scratch/before.mod" -replace="github.com/mellowforge/jsontrans-go=$snapshot"

hash_sources() {
	python3 - "$1" <<'PY'
import hashlib
import pathlib
import sys
root = pathlib.Path(sys.argv[1])
h = hashlib.sha256()
for path in sorted(root.rglob('*.go')):
    rel = path.relative_to(root)
    if path.name.endswith('_test.go') or rel.parts[0] in ('benchmarks', 'examples', '.git', '.superpowers'):
        continue
    h.update(str(rel).encode() + b'\0' + path.read_bytes() + b'\0')
print(h.hexdigest())
PY
}
hash_sources "$snapshot" > "$results/before-source.sha256"
hash_sources ../.. > "$results/current-source-before.sha256"

go test -modfile="$scratch/before.mod" -tags prepared ./...
go test -tags 'prepared batch' ./...
go test -modfile="$scratch/before.mod" -tags prepared -run '^$' \
	-bench '^BenchmarkPrepared$/(1024|8192|65536)/(simple|mixed|unique)/N(4|16|64)/(prepare_only|prepared_reuse|prepare_and_eval)$' \
	-benchmem -benchtime="$duration" -count="$samples" > "$results/before-prepared.txt"
go test -tags 'prepared batch' -run '^$' -bench '^BenchmarkCompiledBatch$' \
	-benchmem -benchtime="$duration" -count="$samples" > "$results/current-batch.txt"
go test -tags prepared -run '^$' \
	-bench '^BenchmarkPrepared$/(1024|8192|65536)/(simple|mixed|unique)/N(4|16|64)/prepare_only$' \
	-benchmem -benchtime="$duration" -count="$samples" > "$results/current-prepare.txt"
hash_sources ../.. > "$results/current-source-after.sha256"
cmp "$results/current-source-before.sha256" "$results/current-source-after.sha256"
python3 summarize-batch.py "$results"
printf 'Results saved to %s\n' "$results"
