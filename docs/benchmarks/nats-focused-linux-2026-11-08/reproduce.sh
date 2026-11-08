#!/usr/bin/env bash
# Linux/Bertram: cached dependencies, Docker and suitable CPU topology required.
# Usage: reproduce.sh REPO ABSOLUTE_GO127 NEW_OUTPUT_DIRECTORY
set -euo pipefail
repo=$(cd "$1" && pwd)
GO=$2
mkdir "$3"
run=$(cd "$3" && pwd)
evidence=$(cd "$(dirname "$0")" && pwd)
export GOTOOLCHAIN=local GOWORK=off GOEXPERIMENT='' GOMAXPROCS=4
export GOPROXY=${GOPROXY:-off}
[[ $("$GO" version) == 'go version go1.27.3 linux/amd64' ]]
mkdir "$run/verification"
{ date -u; uname -a; "$GO" version; lscpu; cat /proc/meminfo; cat /proc/loadavg; docker version; docker ps --format '{{.ID}} {{.Image}} {{.Status}}'; } > "$run/environment.txt"
for version in baseline main; do
 revision=b21df4f8141c69e65bfc1a04c51cb4052e735fb2
 if [[ $version == main ]]; then revision=9e06911e37ac43dadd3c0f3655a3e2961e60ac55; fi
 mkdir -p "$run/$version/source"
 git -C "$repo" archive "$revision" | tar -x -C "$run/$version/source"
 git -C "$run/$version/source" apply "$evidence/harness.patch"
 cd "$run/$version/source/experiments/messages"
 "$GO" test -count=1 ./... > "$run/verification/$version-messages-test.txt" 2>&1
 python3 -B -m unittest discover -p 'test_*.py' > "$run/verification/$version-messages-report-tests.txt" 2>&1
 cd nats
 "$GO" test -count=1 ./... > "$run/verification/$version-nats-test.txt" 2>&1
 "$GO" test -race -count=1 ./... > "$run/verification/$version-nats-race.txt" 2>&1
 "$GO" vet ./... > "$run/verification/$version-nats-vet.txt" 2>&1
 python3 -B -m unittest discover -p 'test_*.py' > "$run/verification/$version-nats-report-tests.txt" 2>&1
 "$GO" test -tags=integration -run '^TestContainerRoutes$' -count=1 -timeout=5m ./... > "$run/verification/$version-nats-integration.txt" 2>&1
 "$GO" build -mod=readonly -o "$run/$version/message-load" ./cmd/load
 "$GO" version -m "$run/$version/message-load" > "$run/$version/build-info.txt"
 sha256sum "$run/$version/message-load" > "$run/$version/binary.sha256"
done
diff -r "$run/baseline/source/experiments/messages" "$run/main/source/experiments/messages" > "$run/harness-diff.txt"
printf 'pair\tversion\toffset\tstart_utc\tend_utc\texit_status\n' > "$run/execution.tsv"
for pair in 1 2 3 4 5 6; do
 order='baseline main';if ((pair%2==0));then order='main baseline';fi
 for version in $order;do
  destination="$run/$version/round-$pair"
  start=$(date -u +%FT%TZ)
  { date -u; cat /proc/loadavg; cat /proc/stat; } > "$run/$version/round-$pair-host-before.txt"
  status=0
  taskset -c 0,2,4,6,8,10,12,14 "$run/$version/message-load" -output "$destination" -rounds 1 -round-offset "$((pair-1))" -duration 10s -profile object-one-worker > "$run/$version/round-$pair.log" 2>&1 || status=$?
  end=$(date -u +%FT%TZ)
  printf '%d\t%s\t%d\t%s\t%s\t%d\n' "$pair" "$version" "$((pair-1))" "$start" "$end" "$status" >> "$run/execution.tsv"
  { date -u; cat /proc/loadavg; cat /proc/stat; } > "$run/$version/round-$pair-host-after.txt"
  if ((status!=0));then tail -12 "$run/$version/round-$pair.log";exit "$status";fi
  python3 -B "$run/$version/source/experiments/messages/nats/summarize.py" "$destination"
  python3 - "$destination" <<'PY'
import json, subprocess, sys
from pathlib import Path
p=Path(sys.argv[1]);m=json.loads((p/'metadata.json').read_text())
rows=[json.loads(s)['report'] for s in (p/'runs.jsonl').read_text().splitlines()]
assert len(rows)==6 and all(r['transform_errors']==0 for r in rows)
assert m['completed'] and m['container_removed'] and not m.get('cleanup_error')
assert not subprocess.check_output(['docker','ps','-aq','--filter','id='+m['container_id']],text=True).strip()
PY
 done
done
cat > "$run/manifest.json" <<'JSON'
{"baseline":"b21df4f8141c69e65bfc1a04c51cb4052e735fb2","main":"9e06911e37ac43dadd3c0f3655a3e2961e60ac55","pairs":6,"profile":"object-one-worker","duration":"10s","go":"go1.27.3","image":"nats@sha256:afae2fdbe2e326c65e1246cb83b9d05d319188734f3bd577ac8072daae78a10f"}
JSON
python3 -B "$repo/experiments/messages/nats/compare.py" "$run"
