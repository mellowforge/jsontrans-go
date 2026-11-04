# JavaScript Extended Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans inline, task by task, then one fresh whole-branch review.

**Goal:** Produce reproducible, longer-duration local soak and resource-boundary fuzz evidence for the optional JavaScript preview.
**Architecture:** Extend only development tooling. Keep the existing count-based hardening command; add a duration mode that reuses the checked workload in bounded batches, records live/closed memory samples and progress, and optionally recycles engines. Add a parameterized resource fuzzer with strict outcome and recovery oracles. Preserve all historical evidence.
**Tech Stack:** Go 1.26.10; existing owned QuickJS/Wazero; local Python and SSH orchestration; existing private Linux/Windows tools.
**Spec:** docs/javascript-hardening.md, Operational and release gates; docs/javascript-execution-contract.md. User instruction “okay, go” authorizes this next validation campaign and the necessary development-harness changes.

## Global Constraints

- Start from merged PR #13, b5e7958ab97371ea5a1fe47ee64bc0db4560b974, on codex/javascript-extended-soak.
- No runtime, public API, guest/source/SDK pin, dependency, execution-profile or release changes.
- No hosted Actions, runner installation, global toolchain changes, merge, tag or release.
- Linux work and caches stay under /data/ci; use existing task-private Go/Node/compiler tools.
- Ordinary count-based CLI behavior and tests remain supported.
- Duration is bounded to two hours, workers to eight; guest caps tested at 2, 8 and 64 MiB. Invalid explicit settings fail.
- Every admitted workload call must have its expected result/error and a healthy recovery. Unexpected deadlines, missing results or false quota provenance fail; never skip them.
- Progress and partial failure evidence are preserved. A crash, timeout, failed observer or incomplete run cannot be called a pass.
- The campaign includes repeated compilation in a long-lived engine, not a compile-once throughput benchmark. Forced-GC measurements are diagnostic, not production latency estimates.

## Review Focus

- Duration expiry versus cancellation: finish a bounded batch normally; an external cancellation or watchdog timeout is a failed/incomplete campaign.
- Progress-write and Close failures: preserve partial counts and return nonzero, draining resources.
- Long-lived versus recycled engines: samples distinguish live retained state from after-Close state; no false leak claim based on Go Sys alone.
- Tiny per-call limits: scalar input/empty bindings must not accidentally move an output-boundary test into the input/bindings phase.
- Fuzz errors: keep native OOM distinct from trusted conversion/limit failures, reject unrelated cancellation, and verify healthy recovery.

### Task 1: Duration-based development soak

**Files:** javascript/cmd/hardening/{main.go,campaign.go,campaign_test.go,main_test.go}.
**Interfaces:** Keep settings/run/exercise/check count mode. Add campaignSettings, runCampaign(ctx, settings, observer) (report,error); report adds optional campaign/status/failure fields. Observer receives bounded progress records. CLI adds -duration, -recycle, -sample-interval, -guest-mib and -progress, used only in duration mode.

- [ ] Add failing tests for settings validation, cancellation and observer failure; successful short campaign must account for every outcome/recovery, contain live and closed samples, and close every constructed engine.
- [ ] Add wrong-quota-provenance OOM checks; watch RED.
- [ ] Implement duration mode in campaign.go, preserving legacy default output fields. Reuse exercise in 25-iteration batches; generate allocation request as five times the configured guest cap (8 MiB preserves the old 40 MiB fixture). No input-dependent JS code generation.
- [ ] Record baseline, live and after-Close GC snapshots with elapsed time; the optional JSONL file is created exclusively, never overwriting evidence. Emit partial final JSON on campaign failure and exit nonzero.
- [ ] CLI duration mode uses an outer watchdog duration+30s and bounded per-call limits. Engine recycle interval zero means persistent; positive intervals recycle after a completed batch. Sample interval defaults 30s. Tests may use short durations.
- [ ] Verify focused tests, public normal/race/vet and a short CLI smoke; commit.

### Task 2: Parameterized resource-boundary fuzzing

**Files:** javascript/resource_fuzz_test.go; .github/workflows/ci.yml; docs/local-verification.md.
**Interfaces:** FuzzOwnedResourceLimits(selector uint8, size uint16, limit uint16). Preconstruct engines at 2/8/64 MiB and immutable array/depth/string/allocation programs plus healthy 42.

- [ ] Use scalar numeric input and empty bindings. Bound generated arrays to 64 items, nesting to 16, strings to 1024 ASCII bytes, and small allocations to 4096 bytes. Big allocation is guest cap+1..65536 bytes and must terminate natively.
- [ ] Check exact collection/depth/encoded-byte success/failure boundaries, no-result/exact-phase/resource provenance, and healthy recovery for every case; no unexpected cancellation is discarded.
- [ ] Prove the oracle detects a wrong collection program with a temporary mutation; restore it and run seeds plus 1000 fuzz executions.
- [ ] Add only a short bounded Linux CI fuzz step and the local command; preserve Mac/Windows runner configuration. No hosted execution.
- [ ] Commit the fuzzer and documentation.

### Task 3: Local campaign and evidence

**Files:** development-only local orchestration; dated results under javascript/testdata/stress/2026-11-04; docs/javascript-extended-validation.md.
**Interfaces:** Consume the committed harness/fuzzer; transfer source bundles with release tags to isolated checkouts. Record source SHA, exact commands/configuration, exit codes, toolchains and artifact hash.

- [ ] Run a one-hour Linux persistent-engine campaign at 8 MiB and a one-hour recycled-engine campaign at 64 MiB, four workers each, GOMAXPROCS=4 per process. Recycle every minute; sample every 30s. Keep individual process RSS monitoring separate from Go heap/guest caps.
- [ ] Run 15-minute mixed-lifecycle checks on Mac and native Windows, two workers each, using the same committed source. No cross-build is recorded as native execution.
- [ ] On Linux, extend owned compile/diagnostic/location/string/JSON/resource fuzz campaigns with finite execution counts and explicit test watchdogs. Preserve failing corpus entries and investigate any failure; never relabel a retry as the original clean run.
- [ ] Evaluate post-warmup retained-heap/goroutine trends separately for live and closed samples. Report endpoints, maxima, time windows and observations; unexplained growth is investigated, not hidden behind a generic PASS.
- [ ] Refresh all-module formatting/tests/race/vet, packaging/consumer, owned probes and relevant native compatibility checks. Run current scanner matrix if code coverage changes. No production approval follows automatically.
- [ ] Record actual completed durations/counts; retain aborted/incomplete results. Commit new evidence separately from PR #13 snapshots.

### Task 4: Review and handoff

**Files:** final campaign record and evidence notices as needed.
**Interfaces:** One fresh whole-branch reviewer after implementation/measurements. Fix Important/Critical findings in one regression-driven pass; document scope and deferred minors.

- [ ] Independently review harness lifecycle, duration/cancellation, IO failure accounting, resource oracles and evidence claims.
- [ ] Verify any fixes with RED→GREEN and whole suites. Update evidence if behavior changes.
- [ ] Publish a draft PR with a [skip ci] tip after all required work passes; no merge/tag/release. Keep private reporting and production approval as separate gates.
