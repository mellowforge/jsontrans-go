# Windows memory diagnostic evidence — 2026-11-04

Source: merged main `8e7e84c9e53d89923658da9139d797575ed745fe`.
The [investigation report](../../../../docs/javascript-windows-memory.md)
explains the findings and limitations. These are bounded diagnostic experiments,
not benchmarks or new production limits. Nothing here changes the guest,
production dependency graph, public API or CI workflows.

## Contents

- `builds.json`: Go build information and SHA-256 identities for four executables
  (executables are not committed). Built on macOS/arm64 with Go 1.26.10,
  GOOS=windows, GOARCH=amd64, CGO_ENABLED=0; executed natively on Windows (recorded OS build
  `Microsoft Windows NT 10.0.22621.0`).
- `probe1`: the initial failed PowerShell invocation. Unquoted dotted test flags
  were parsed incorrectly (`-test.run` became `-test`); the command exited before
  writing `exit.json`. It is not a passing test and is excluded from analysis.
- `probe2`: corrected invocation, 40 lifecycles, profiling after first-GC spikes.
  The 20 binary heap profiles are sampled allocation evidence. Four `*.top.txt`
  files retain `go tool pprof -top -nodecount=8 -sample_index=inuse_space` output.
- `baseline80`: 80 lifecycles without profile writing, original Wazero.
- `wait80`: identical diagnostic source/settings, with only the temporary
  Wazero monitor-join control. No profiling or scheduler yield in either run.
- `stacks10`: ten post-Close snapshots of runnable monitors, GOMAXPROCS=1 set
  inside the test. Runner process starts with GOMAXPROCS=2; this test override
  intentionally amplifies the scheduling boundary.
- `sources`: exact diagnostic sources stored as `.go.txt` to avoid compiling
  them as ordinary package tests, PowerShell runners and the control patch.
- `analysis.json`: output of `python3 analyze.py`; validates exit codes,
  accounting, sample order and the recorded control result before summarizing.
- `SHA256SUMS`: hashes of every evidence file except the manifest itself.

PowerShell-generated logs retain their original UTF-16 BOM/CRLF; JSON metadata
may have a UTF-8 BOM. `analyze.py` handles both. The first probe runner was
corrected by quoting complete dotted flags (`'-test.run=…'`); both scripts are
retained. The probe2 source is reconstructed by reversing the subsequently
added pre-GC counters/optional flags and 80-cycle setting; its behavior matches
the original 40-cycle diagnostic. The exact 80-cycle source is archived directly.

`run*.json` structured command arrays describe equivalent test arguments; the
runner scripts contain the actual quoting. `run.json` and `exit.json` retain
start/end times and native process exit codes. Run durations are also in test
logs. The memory diagnostics use a five-minute context and a six-minute Go test
watchdog. The stack probe uses background operation contexts and is bounded by
its six-minute Go test watchdog. The later probes retained all earlier outputs.

## Reproduce in disposable source copies

Export the source commit into a temporary checkout, leaving your working tree
unchanged. To reproduce either 80-cycle run, copy
`sources/memory_diagnostic_test.go.txt` into
`javascript/cmd/hardening/memory_diagnostic_test.go`. From that copy:

```sh
GOTOOLCHAIN=go1.26.10 GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go -C javascript test -c -o diagnostic2.test.exe ./cmd/hardening
```

For the wait control, copy Wazero v1.12.0 from the module cache to a separate
writable directory. Apply `sources/wazero-monitor-wait.patch` there with
`git apply`, then add a replacement **only in the disposable source copy**:

```sh
go -C javascript mod edit -replace github.com/tetratelabs/wazero=/absolute/path/to/control-copy
GOTOOLCHAIN=go1.26.10 GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go -C javascript test -c -o diagnostic-wait.test.exe ./cmd/hardening
```

Transfer the executables and runner scripts to Windows. The archived runners
use `$env:USERPROFILE\jsontrans-ci\winmem-20261104`; change that task directory
or supply fresh `-name` output directories to avoid overwriting evidence:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File run3.ps1 -name baseline80-new -yieldMode 0
powershell -NoProfile -ExecutionPolicy Bypass -File run-wait.ps1 -name wait80-new -yieldMode 0
```

The two controls use the same diagnostic source, GOMAXPROCS=2, 64 MiB guest cap,
two workers and 25 iterations per lifecycle. `JSONTRANS_MEMORY_PROFILE=0` and
`JSONTRANS_MEMORY_YIELD=0` in both. All first/subsequent GC observations are
retained; first-GC spikes are scheduler-dependent and need not occur on every
rerun. The analyzer asserts facts about the archived experiment, not universal
thresholds for future platforms or runs.

For the profiling probe, use `sources/profile_memory_diagnostic_test.go.txt`
instead and `sources/run.ps1`. For the stack probe use unmodified Wazero, add
`sources/monitor_diagnostic_test.go.txt` under its `.go` filename, build as
`diagnostic-stacks.test.exe` and use `sources/run-stacks.ps1`. No Node, CGO or
network access is needed to execute the diagnostics; building needs the pinned
Go modules/toolchain already available.

The temporary Wazero patch derives from v1.12.0's
`internal/wasm/module_instance.go`, licensed under Apache-2.0. The upstream
license and notices are preserved as `sources/WAZERO-LICENSE` and
`sources/WAZERO-NOTICE`. The patch is not a production
backend and must not be used to infer release approval or a memory guarantee.
