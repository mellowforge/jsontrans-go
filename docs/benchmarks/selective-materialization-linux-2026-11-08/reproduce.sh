#!/usr/bin/env bash
# Usage: reproduce.sh REPO ABS_GO127 ABS_GO126 NEW_OUTPUT_DIRECTORY
# Linux; taskset CPU 8 must be available. Dependencies must already be cached.
set -euo pipefail
repo=$(cd "$1" && pwd)
go127=$2
go126=$3
mkdir "$4"
out=$(cd "$4" && pwd)
evidence=$(cd "$(dirname "$0")" && pwd)
base=f533e6020b5321d4c506a48ae481df8c9a523661
export GOTOOLCHAIN=local GOWORK=off GOEXPERIMENT='' GOMAXPROCS=1
export GOPROXY=${GOPROXY:-off}
for version in baseline candidate; do
 mkdir -p "$out/$version/source"
 git -C "$repo" archive "$base" | tar -x -C "$out/$version/source"
done
git -C "$out/candidate/source" apply "$evidence/candidate.patch"
for version in baseline candidate; do
 "$go127" -C "$out/$version/source/experiments/messages" test -mod=readonly .
 "$go127" -C "$out/$version/source/experiments/messages" test -mod=readonly -c -o "$out/$version/messages.test" .
 "$go126" -C "$out/$version/source/experiments/messages" test -mod=readonly -c -o "$out/$version/messages-go126.test" .
done
# Compile once; alternate binary order and rotate strategy order across rounds.
for toolchain in go127 go126; do
 rounds=5; duration=300ms; sizes='(1024|65536)'; binary=messages.test; folder=paired
 if [[ $toolchain == go126 ]]; then
  rounds=3; duration=200ms; sizes='^1024$'; binary=messages-go126.test; folder=go126
 fi
 for ((round=1; round<=rounds; round++)); do
  order='baseline candidate'; if ((round%2==0)); then order='candidate baseline'; fi
  for version in $order; do
   mkdir -p "$out/$version/$folder"
   cd "$out/$version/source/experiments/messages"
   MESSAGE_STRATEGY_ORDER=$(((round-1)%3)) taskset -c 8 "$out/$version/$binary" \
    -test.run '^$' -test.bench "^BenchmarkBytes$/(jsonata|jsonpath)/(scalar|object)/$sizes/(N1|N16)/(direct|prepared|batch)$" \
    -test.benchmem -test.cpu=1 -test.benchtime="$duration" > "$out/$version/$folder/round-$round.txt" 2>&1
  done
 done
done
# Diagnostic profiles are separate from timing samples. Their ns/op is unusable
# for latency comparisons, especially with the allocation sampling rate of 1.
for version in baseline candidate; do
 cd "$out/$version/source/experiments/messages"
 mkdir -p "$out/$version/profiles"
 for engine in jsonata jsonpath; do
  for kind in object; do
   for count in 1 16; do
    strategy=prepared; if ((count==16)); then strategy=batch; fi
    name="$engine-$kind-N$count-$strategy"
    # Anchor EVERY component; N1 must not also match N16.
    printf -v regex '^BenchmarkBytes$/^%s$/^%s$/^1024$/^N%s$/^%s$' "$engine" "$kind" "$count" "$strategy"
    stem="$out/$version/profiles/$name"
    taskset -c 8 "$out/$version/messages.test" -test.run '^$' -test.bench "$regex" -test.benchmem -test.cpu=1 -test.benchtime=2s -test.cpuprofile "$stem.cpu.pprof" > "$stem.cpu-bench.txt" 2>&1
    taskset -c 8 "$out/$version/messages.test" -test.run '^$' -test.bench "$regex" -test.benchmem -test.cpu=1 -test.benchtime=1s -test.memprofile "$stem.alloc.pprof" -test.memprofilerate=1 > "$stem.alloc-bench.txt" 2>&1
    "$go127" tool pprof -top -nodecount=25 "$out/$version/messages.test" "$stem.cpu.pprof" > "$stem.cpu-top.txt"
    "$go127" tool pprof -top -cum -focus='document.Node.materialize' -nodecount=15 "$out/$version/messages.test" "$stem.cpu.pprof" > "$stem.cpu-materialize.txt"
    "$go127" tool pprof -alloc_space -top -nodecount=25 "$out/$version/messages.test" "$stem.alloc.pprof" > "$stem.alloc-space.txt"
    "$go127" tool pprof -alloc_objects -top -nodecount=25 "$out/$version/messages.test" "$stem.alloc.pprof" > "$stem.alloc-objects.txt"
    if "$go127" tool pprof -alloc_space -list 'document.Node.materialize' "$out/$version/messages.test" "$stem.alloc.pprof" > "$stem.materialize.txt" 2> "$stem.materialize.err"; then
     rm "$stem.materialize.err"
    elif grep -Fxq 'no matches found for regexp: document.Node.materialize' "$stem.materialize.err"; then
     echo 'No sampled allocations attributed to document.Node.materialize in this profile.' > "$stem.materialize.txt"
     rm "$stem.materialize.err"
    else
     cat "$stem.materialize.err" >&2; exit 1
    fi
   done
  done
 done
done
