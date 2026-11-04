# QuickJS PR B evidence — 2026-11-04

Runtime source tested: `4f449c247c064fa071f6c531d52b77f3c1495e9e` on
`codex/quickjs-terminal-errors`; base is merged PR #12
`3fc979a0d97d93cde0b92f6bb6dc7a6bdf244c2a`. Later changes add this evidence,
current documentation and corrected descriptive probe text. No historical audit
or measurement file is replaced. No hosted Actions run, module tag or release
is part of this evidence.

See [the upgrade record](../../../../docs/javascript-quickjs-upgrade.md) for
pins, the approved OOM patch, ABI3 and public behavior. See
[source-review.md](source-review.md) for the C advisory review. Go scanning is
recorded separately and does not analyze the embedded engine.

## Platform results

- Mac arm64 / Apple M5 Max: Go 1.26.10 tests/race/vet in all three modules;
  Go 1.27.3 native tests/race/vet; formatting; packaging regressions and outside
  consumer; 21 owned probes; focused runtime fuzz and independent guest rebuild.
- Linux amd64 / Ubuntu 24.04 / Intel i9-13900K: all nine groups in
  `linux-verification.json` passed. Includes both native Go versions, all three
  modules, Node 22 oracle, all ten fuzz targets, packaging, 21 probes, three
  security targets and a separately rebuilt, byte-identical guest. Caches and
  private toolchains live under `/data/ci`.
- Native Windows x64 VM: Go 1.26.10 public-module tests/race/vet/module verification
  and CGO-free application build/run pass, as recorded in
  `windows-verification.json`. The actual NT build is 10.0.22621.0. The private
  compiler is WinLibs GCC 16.2.0 with MinGW-w64 UCRT 14.0.0, checked from the
  release asset digest, with `libsynchronization.a` available for Go's race
  runtime. No global PATH or system toolchain was changed.

The saved `linux-driver.py` and `windows-driver.ps1` are the task-local command
orchestration, not installed runner services. Paths identify the original
private workspaces and should be adapted to reproduce elsewhere. Both use
complete source history plus v1.0.0/v1.1.0 tags for packaging. Reports retain
commands, durations, exit codes and source identity. Compressed logs preserve
output; successful vet/module/compare commands may emit no output. The first
Windows attempt stopped on PowerShell 5.1 treating Go download stderr as an
error; process-based logging fixed the harness before the recorded clean run.

The original Linux driver used the same log filename for the two native
CompileAndEval fuzz targets: both passed in the report, but the JSONata output
replaced the JSONPath output. `mac-jsonpath-fuzz.txt.gz` separately repeats the
40,000-execution JSONPath check with its own retained log. Other target logs are
unique. This evidence does not silently drop a target.

## Recovery and native lifetime evidence

`linux-soak-100k.json` records 100,000 failure/success calls, each followed by a
verified healthy evaluation, over five engine lifecycles and four workers at an
8 MiB cap. It ran for 156.46 seconds. Each failure category (ordinary exception,
conversion, cancellation, allocation) and the success category ran 20,000 times.
Post-GC goroutines return to one; after the first cycle, retained Go heap stays
near 0.5 MiB and Go Sys stays at 74,934,552 bytes. These are observations from a
bounded run, not hard process-memory guarantees or prolonged deployment proof.
The earlier 10,000-call run is also retained.

Native ASan/UBSan/leak checks exercise twelve OOM/context/GC cycles and 61
partial initialization limits; see the checked-in development harness and the
upgrade record's exact compiler command. Omitting the GC-root mark fails the
retained-object assertion, and omitting the trusted bridge phase update fails
two terminal fuzz seeds. Expected failing mutation/old-guest outputs are named
`*-red` or `*-mutation`; they are not clean-suite runs. Compiler warnings are
retained without suppression. No upstream native exploit/RCE payload is run.

## Measurements and reproduction

`before-bench.txt`, `after-bench.txt`, the two cold JSON files and
`measurements.json` retain matched Go 1.26.10 / Apple M5 Max / single-P runs.
The baseline binaries were verified to contain the exact old guest bytes before
measurement. The before benchmark's trailing module-cache permission warning
is retained; all benchmark samples completed with PASS. New timings use the
combined source/patch/phase change and do not isolate any one component.

```sh
GOTOOLCHAIN=go1.26.10 GOMAXPROCS=1 go -C javascript test -run '^$' \
  -bench '^BenchmarkJavaScript$' -benchmem -benchtime=200ms -count=5 -cpu=1
CGO_ENABLED=0 go -C javascript build -o /tmp/jsontrans-example ./examples/basic
CGO_ENABLED=0 go -C experiments/javascript build -o /tmp/jsontrans-assess ./cmd/assess
python3 experiments/javascript/measure_cold.py --binary /tmp/jsontrans-assess \
  --backend owned-quickjs --output /tmp/jsontrans-cold.json
go -C javascript run ./cmd/hardening -iterations 5000 -workers 4 -cycles 5
```

Cold RSS is a process high-water observation, Go B/op is allocation traffic,
and guest caps describe linear memory: these are distinct quantities. Neither
short benchmark samples nor this recovery soak establishes production approval.
For the full repeatable checks, use [local verification](../../../../docs/local-verification.md).
