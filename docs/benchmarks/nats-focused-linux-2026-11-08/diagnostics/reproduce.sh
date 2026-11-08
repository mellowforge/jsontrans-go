#!/usr/bin/env bash
set -euo pipefail
run=$(cd "$1" && pwd)
GO=$2
export GOTOOLCHAIN=local GOWORK=off GOPROXY=${GOPROXY:-off} GOEXPERIMENT='' GOMAXPROCS=4
evidence=$(cd "$(dirname "$0")" && pwd)
cp "$evidence/trace-runner.go.txt" "$run/diagnose.go"
# Diagnostic tools/builds run only after all primary latency runs complete.
python3 - "$run" <<'PYCHK'
import csv, json, sys
from pathlib import Path
p=Path(sys.argv[1])
rows=list(csv.DictReader((p/'execution.tsv').open(),delimiter='\t'))
assert len(rows)==12 and all(r['exit_status']=='0' for r in rows)
for pair in range(1,7):
 for v in ['baseline','main']:
  m=json.loads((p/v/f'round-{pair}'/'metadata.json').read_text())
  assert m['completed'] and m['container_removed']
PYCHK
"$GO" build -o "$run/go-trace" cmd/trace
for version in baseline main; do
 cd "$run/$version/source/experiments/messages/nats"
 "$GO" build -o "$run/$version/diagnose" "$run/diagnose.go"
 "$GO" version -m "$run/$version/diagnose" > "$run/$version/diagnose-build-info.txt"
 sha256sum "$run/$version/diagnose" > "$run/$version/diagnose.sha256"
done
for case in 1-prepared 4-prepared 4-batch; do
 fanout=${case%-*}; strategy=${case#*-}
 for version in baseline main; do
  out="$run/diagnostics/$version-$case"
  mkdir -p "$run/diagnostics"
  taskset -c 0,2,4,6,8,10,12,14 "$run/$version/diagnose" -output "$out" -duration 5s -fanout "$fanout" -strategy "$strategy" > "$run/diagnostics/$version-$case.log" 2>&1
  for kind in sched sync net; do
   "$run/go-trace" -pprof="$kind" "$out/trace.out" > "$out/$kind.pprof"
   "$GO" tool pprof -top -nodecount=20 "$run/$version/diagnose" "$out/$kind.pprof" > "$out/$kind-top.txt"
  done
  echo "DIAGNOSTIC $version $case COMPLETE"
 done
done
