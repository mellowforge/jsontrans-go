# Linux reproduction

Measured on Bertram, Ubuntu 24.04 (VERSION_ID independently verified after the
run), amd64/i9-13900K, from clean commit
`dbd3057f96cf2ecaa1e31816255407132a3cc8a6`. Both Linux datasets and both prior
Mac datasets match the Go/module source hash recorded in metadata.json. Reports
were collected from a task-private directory under /data/ci after the driver
finished with exit code zero and confirmed that its source checkout was clean.

The driver ran the 21 commands in [verification/results.json](verification/results.json)
with GOMAXPROCS=8, GOTOOLCHAIN=local, GOWORK=off, GOFLAGS empty, and
GOPROXY=off. Existing module/toolchain/build caches lived under /data/ci. It
checked formatting and the Python summarizer, then ran the benchmark runner.
All verification logs are retained adjacent to the manifest. No tests ran
concurrently with timing.

Before benchmarking, the driver set its own CPU affinity to logical CPU8; its
children inherited that mask. It did not alter system-wide affinity/governors.
The benchmark runner then set GOMAXPROCS=1/-test.cpu=1 as usual. Host settings,
including the powersave governor and unlocked frequency, are in host.json.

Equivalent benchmark commands on Bertram, with GO126/GO127 pointing to the
installed native bin/go executables and a writable, already cached Go environment:

```sh
GOTOOLCHAIN=local GOWORK=off GOPROXY=off \
  taskset -c 8 python3 experiments/go127json/run.py \
  --go126 "$GO126" --go127 "$GO127" \
  --output /tmp/go127-json-linux-results --count 6 --benchtime 150ms
python3 experiments/go127json/summarize.py /tmp/go127-json-linux-results

GOTOOLCHAIN=local GOWORK=off GOPROXY=off \
  taskset -c 8 python3 experiments/go127json/run.py \
  --go126 "$GO126" --go127 "$GO127" \
  --output /tmp/go127-json-linux-prepared-control --count 6 --benchtime 1s \
  --skip-javascript \
  --native-pattern 'BenchmarkJSONAPI$/64KiB$/prepare_batch$/N16$'
python3 experiments/go127json/summarize.py /tmp/go127-json-linux-prepared-control
```

Set task-local GOMODCACHE, GOCACHE, GOPATH, TMPDIR and GOTMPDIR as appropriate;
no privileged installation or service is needed. The output directories must be
empty. Choose an available consistent CPU on a different machine rather than
assuming CPU8 is suitable. See the [runner guide](../../../experiments/go127json/README.md)
and [Linux assessment](../../assessments/go127-json-linux.md).
