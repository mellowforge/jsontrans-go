# Final error-propagation correction and verification

This addendum supersedes the pre-review guest results in the parent directory.
Their original measurements, reports and checksums remain intact. Runtime source
for this run is `44b6086891ca947bd6446c00d080145d93ebbcb7`. Subsequent changes
only record documentation and evidence; no runtime source changes follow it.

Final owned Wasm: 1,034,164 bytes, SHA-256
`31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee`.
Owned patch SHA-256:
`41e04680ad186ec1d230874e949593bf0723c443c31966941509e25461eb146b`.
Upstream commit/archive, SDK 34.0 and ABI3 remain pinned as in the upgrade record.

## Independent review and regression evidence

The whole-branch reviewer found one Important issue: a partially constructed
ordinary error could replace pending terminal OOM; Error.prepareStackTrace
could similarly mask it while restoring an earlier exception. The executor
reproduced both before fixing them. Three public stack cases returned a value
and an output case lost the native OOM cause. A native message-allocation sweep
also failed. After partial error propagation was fixed, a further bounded test
showed that a formatter could run until timeout after terminal stack-limit
conversion; the final patch stops that callback before it runs.

The correction propagates message/property failures, preserves terminal errors
on saved-exception restoration, propagates stack-construction failure through
its object-returning callers, and releases partial values. Ordinary formatting
exceptions retain upstream behavior. It adds no host import or public API.
The reviewer was not asked for a second review: the single fix pass is verified
by failing-then-passing regressions and complete suites. It is not represented
as a new reviewer approval.

The expanded native harness sweeps 513 allocation limits around TypeError
construction, requiring either a complete ordinary message or a terminal error.
It then checks twelve OOM/context/GC cycles and 61 partial-initialization limits.
Linux ASan/UBSan/leak checks and the Mac native build pass without source
instrumentation. The known missing-GC-root mutation proof remains in the parent
directory. No memory-corruption/advisory exploit is needed for these checks.
The public suite retains a 2 MiB normal/error/recovery regression and ordinary
stack formatter controls; the terminal fuzz target now includes formatting paths.

## Local verification

The final Linux and Windows JSON reports identify the source above and retain
per-command exit codes and durations. Mac output is preserved separately.
Linux repeats all nine groups, with unique filenames for both native fuzz
packages this time, and a 100,000-pair recovery soak. Mac repeats all three
modules' tests/race/vet and consumer packaging; Windows repeats native public
vet/tests/race/module verification and the CGO-free example. Both rebuild hosts
produce the final recorded guest bytes. The three-target Go source/binary scan
is refreshed as well. Native Go 1.27.3 was unchanged and already passed on Mac;
Linux's full rerun also includes that toolchain.

## Matched final measurements

The parent `before-bench.txt` / `before-cold.json` remain the old-guest baseline.
This directory's `after-bench.txt`, `after-cold.json` and `measurements.json`
measure the final guest with the same Go 1.26.10, Apple M5 Max, GOMAXPROCS=1,
five 200 ms benchmark samples and five independent cold processes. The parent
`after-*` measurements describe the pre-review guest, not the final correction.
Use the exact commands in the parent README, directing output to new filenames.

All code remains a preview. Full advisory exploit reproduction and a complete
engine audit were not performed; the source-review table is explicit about its
scope. The automated filter interrupted the reviewer's first turn; the same
reviewer completed static inspection and existing checks, and the executor used
bounded correctness regressions for the confirmed finding. Production approval,
prolonged deployment observation and a monitored private reporting route remain
separate release decisions. No hosted Actions, merge, tag or release is included.
