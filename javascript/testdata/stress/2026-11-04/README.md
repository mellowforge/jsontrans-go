# Extended JavaScript validation — 2026-11-04

Runtime/harness/fuzzer source: `dc29b608edb42111221fa3e173e889b9a7f61583`,
from merged PR #13 `b5e7958ab97371ea5a1fe47ee64bc0db4560b974`.
Guest bytes remain SHA-256
`31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee`.
This work changes development tooling/tests, not engine code or dependencies.
Subsequent commits record documentation and evidence. No hosted Actions, module
release/tag, merge or repository security-setting change is performed.

See the [campaign protocol](../../../../docs/javascript-extended-validation.md)
for workload limits, CLI commands and interpretation. Original PR #13 evidence
is retained under `javascript/testdata/security/2026-11-04-quickjs`.

## Evidence layout

- `mac/`: accepted native Mac arm64 15-minute recycled campaign, per-process
  RSS samples, final report and all-module verification/packaging output.
- `windows-original/`: first Windows attempt. Native vet/tests/race/mod/build
  passed; the Go child reported a complete 15-minute run with 189,950 verified
  recoveries. PowerShell's asynchronous process wrapper lost the exit code, so
  the external runner correctly marked this attempt failed. It stays failed.
- `windows/`: separate accepted rerun with cached process Handle, using the
  identical verified native binary. The report names the original native
  verification it reuses and records the binary SHA. It does not relabel the
  first attempt or pretend its native checks were executed again.
- `interruption-control/`: deliberately terminated short Mac child. Nonzero
  exit, incomplete status, partial verified counts and engine cleanup are the
  expected successful control assertions. Its campaign report itself is failed.
- `campaign-runner.py`, `linux-driver.py` and both PowerShell drivers are the
  exact task-local orchestration, with original absolute paths for provenance.
  Adapt paths for reproduction; no runner service or global toolchain is needed.
- `analyze.py`: reproducible diagnostic summaries, requiring an accepted runner
  and complete child report, with exact per-outcome/recovery accounting. To use
  on archived directories, decompress `rss.jsonl.gz` first. It does not turn
  diagnostics into a general leak/memory-safety proof.

## Measurement notes

Go 1.26.10 is used throughout. Linux profiles use four workers/GOMAXPROCS=4 per
process, at 8 MiB persistent and 64 MiB recycled guest caps. Mac uses two
workers/GOMAXPROCS=2 at 8 MiB; Windows uses two at 64 MiB. Recycling occurs every
60 seconds; forced-GC memory samples occur at baseline, live state and Close.
The workload recompiles five expressions in each 25-iteration batch and checks
success, native exception, conversion, cancellation and allocation outcomes;
every admitted successful campaign iteration has a checked healthy recovery.

The report retains timing/sample arrays, so small growth in Go heap includes
measurement storage. Analysis excludes the first ten percent as warmup and
compares up to five observations in each stage per end window,
with the actual window count in each JSON summary. Persistent engines have only
one final closed sample, which cannot establish a closed-state trend.
Go Sys, retained Go heap, sampled process RSS and guest caps are different
quantities. No hard process-memory bound is claimed. No throughput comparison
is inferred under shared machine load and forced GC.

The Windows fixed run has intermittent after-Close snapshots around 3.9 MiB
alongside 0.3–0.4 MiB snapshots; later snapshots return to the lower level.
Those residual snapshots are retained, including the higher final observation.
There is no monotonically increasing sequence across them and sampled RSS
finishes below its post-warmup starting window. The precise retention source
was not isolated; Close is not a guarantee of immediate physical reclamation.
The source contract and raw maximum/endpoint data remain visible.

JSONL/long logs are gzip-compressed without rewriting their payloads. Windows
JSON retains its original BOM/CRLF. No native executables or toolchain archives
are committed. `SHA256SUMS` is generated after all campaign results are complete.

## Completion status

All four accepted profiles completed with reliable zero exit codes. Linux
persistent/recycled profiles ran 3600.061/3600.104 seconds and verified
1,394,900/1,389,500 recovery pairs. Mac verified 217,400 and corrected Windows
195,300 over 15 minutes each. Completed checks include all seven extended fuzz
targets, all-module tests/race/vet, Node oracle, consumer packaging, 21 probes
and three-target Go security scans. See `linux/report.json` and per-profile
`analysis.json` for actual commands, exit codes and window/max data.
`continuity.json` exposes sample coverage and maximum sampling gaps.
