# JavaScript processor feasibility and cost assessment

User-approved scope: optional CGO-free synchronous function-expression processors,
QuickJS/Wazero primary, Goja integration baseline, no fallback to weaker guarantees.
The root processor/program/registry interfaces and dependency graph stay unchanged.
User steering also authorizes Go 1.26 minima and baseline/new release preparation.

## Execution tasks

- [x] Pin both candidates and isolate the assessment in `experiments/javascript`.
- [x] Audit WebAssembly imports, filesystem settings, configuration caches, cleanup,
      compilation, context cancellation and guest allocation controls.
- [x] Add regression-first JSON transformation and conversion coverage, then implement
      experimental processors with fresh runtime state and immutable compiled artifacts.
- [x] Run destructive/hanging probes in deadline-controlled subprocesses, record every
      failing gate explicitly, and test recovery, state isolation and concurrency.
- [x] Measure compilation, first use, evaluation/conversion, prepared fallback, sequential
      expressions, allocations, guest/process memory, binary size and build requirements.
- [x] Publish pinned evidence, backend recommendation, revised cost and a production plan.
- [x] Run root and assessment checks, focused fuzzing and one fresh whole-change review.

## Acceptance

All assessment observations must be reproducible with pinned modules and commands.
A candidate may fail a gate; the assessment must report it rather than weaken the
required profile or call the experimental adapter production-ready. Protect probe
runners with external process deadlines. Do not run uncapped allocation probes on
Goja. No async, imports, Node APIs, host callbacks, runtime pooling, releases or pushes.

The JSON bridge retains missing/null, finite binary64 numeric behavior, lossless
UTF-16 where supported, native diagnostics, detached caller data and bounded
conversion work/depth/collection/bytes. Unsupported explicit work/regex limits must
be rejected. Test compatibility with the root registry's resolved-option forwarding.

## Review focus

Cancellation and cleanup after deadline expiry; global configuration contamination;
accessors/proxies/toJSON during conversion; guest vs total process memory; lossless
Unicode and unsupported result values; no missing gate outcomes or misleading metrics.
