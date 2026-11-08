#!/usr/bin/env bash
set -euo pipefail
run=$(cd "$1" && pwd)
GO=$2
repo=$(cd "$3" && pwd)
export GOTOOLCHAIN=local GOWORK=off GOPROXY=${GOPROXY:-off} GOEXPERIMENT='' GOMAXPROCS=4
evidence=$(cd "$(dirname "$0")" && pwd)
cp "$evidence/control-runner.go.txt" "$run/diagnose-control.go"
cp "$evidence/runtime-procs.patch" "$run/jsontrans-nats-procs-diagnostic.patch"
cp "$repo/experiments/messages/nats/procs_integration_test.go" "$run/procs_integration_test.go"
for version in baseline main;do
 mkdir -p "$run/controls/$version"
 cp -a "$run/$version/source" "$run/controls/$version/source"
 git -C "$run/controls/$version/source" apply "$run/jsontrans-nats-procs-diagnostic.patch"
 cd "$run/controls/$version/source/experiments/messages/nats"
 cp "$run/procs_integration_test.go" ./
 "$GO" test -tags=integration -run '^TestOneWorkerWithIndependentRuntimeProcs$' -count=1 ./... > "$run/controls/$version/integration.txt" 2>&1
 "$GO" build -o "$run/controls/$version/diagnose" "$run/diagnose-control.go"
 "$GO" version -m "$run/controls/$version/diagnose" > "$run/controls/$version/build-info.txt"
 sha256sum "$run/controls/$version/diagnose" > "$run/controls/$version/binary.sha256"
done
for round in 1 2 3;do
 case $round in
  1) order='baseline-1 main-1 main-2 baseline-2';;
  2) order='baseline-2 main-2 main-1 baseline-1';;
  3) order='main-1 baseline-1 baseline-2 main-2';;
 esac
 for item in $order;do
  version=${item%-*};procs=${item#*-}
  out="$run/controls/$version/round-$round-procs-$procs"
  taskset -c 0,2,4,6,8,10,12,14 "$run/controls/$version/diagnose" -output "$out" -duration 5s -fanout 4 -strategy prepared -procs "$procs" -trace=false > "$out.log" 2>&1
  echo "CONTROL round=$round $version procs=$procs COMPLETE"
 done
done
