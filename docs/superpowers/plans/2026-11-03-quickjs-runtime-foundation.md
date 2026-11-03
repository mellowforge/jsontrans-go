# Owned QuickJS runtime foundation implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Prove an owned QuickJS/Wazero guest can enforce the required resource and capability controls without adopting the QJS wrapper.

**Architecture:** Keep the work in `experiments/javascript`. Build a pinned QuickJS-ng core and small project-owned C ABI into Wasm, with a checked-in artifact and repeatable build. Own the Wazero runtime/configuration per engine, compile immutable bytecode once, and instantiate independent guest state for each compilation/evaluation. Discard guest modules directly on failure or cancellation.

**Tech Stack:** Go 1.26.10, Wazero 1.9.0, QuickJS-ng `d01ca4491fb24ccfeccb4c7394e28a3b21fd5986`, WASI SDK 34.0, no CGO in application builds.

**Spec:** `docs/javascript-assessment.md`, production stage1 and acceptance gates, approved in chat. Existing assessment adapter/bridge contracts remain the baseline.

## Global constraints

- Experimental optional module only; no production processor release or root API/dependency changes.
- Default guest linear-memory ceiling 64 MiB; distinguish from QuickJS heap accounting and host/process RSS.
- No guest filesystem/network, Go callbacks, runtime imports, Node APIs, async queue or runtime pooling.
- Cancellation applies inside guest source compilation, JS loops, built-ins, conversion and exception formatting.
- Keep trusted bridge compilation separate from source; never mutate caller-owned JSON or share mutable guest state.
- Explicitly reject unsupported controls; preserve known registry resolved-limit issue until separately designed.
- Reproduce artifacts from checksum-pinned source/toolchain, retain licenses, audit imports and ABI.

## Review focus

- A canceled/trapped guest is discarded without guest cleanup using the canceled context.
- Concurrent independent engines cannot contaminate cancellation or memory configuration.
- Unexpected imports, missing exports or changed artifact bytes fail closed.
- Source injection and prototype mutation cannot replace the independent trusted conversion bridge.
- Memory exhaustion and adversarial error formatting permit later evaluations in fresh instances.

### Task 1: Own the guest artifact

Files: `experiments/javascript/internal/quickjswasm/guest/{abi.c,build.py,lock.json,LICENSE.quickjs}`, generated `quickjs.wasm`, artifact tests.

- [x] Add artifact tests: exact hash and versioned ABI, strict import allowlist, memory/exports expectations.
- [x] Observe failure before artifact generation.
- [x] Download checksum-pinned source and SDK; build only QuickJS core plus minimal C ABI, excluding quickjs-libc and wrapper proxy/module APIs.
- [x] Build twice in separate directories; compare bytes, record hash, retain upstream license.
- [x] Verify artifact tests and document host build requirements separately from application builds.

### Task 2: Own runtime controls and adapter integration

Files: `internal/quickjswasm/{runtime.go,runtime_test.go}`, assessment `processor.go`, `quickjs_owned.go`.

Produces: `New(ctx, Config) (*Engine,error)`, `Engine.Compile(ctx,string)([]byte,error)`, `Engine.Eval(ctx,rule,bridge []byte,input,bindings string,Limits)(string,error)`, `Engine.Close() error`.

- [x] Regression-first tests for immutable bytecode, fresh/concurrent state, independent engine caps, canceled compilation/execution, clean Close and recovery.
- [x] Implement owned runtime and cache lifetime, always-on context termination, explicit page cap, no filesystem or callback registration, bounded ABI copies and module disposal.
- [x] Add an `owned-quickjs` assessment backend using the existing strict bridge; preserve baseline candidates for comparison.
- [x] Run contract, input ownership, strict result, Unicode, injection and explicit-control tests across the new backend.

### Task 3: Rerun gates and measure checkpoint

Files: `probes.go`, `run_probes.py`, `benchmark_test.go`, results under `testdata/results/owned`, documentation and CI.

- [x] Add new backend to deadline-controlled probes, including hard guest cap, unhandled exception coercion, cleanup and concurrent engine configuration.
- [x] Record every remaining failure without weakening requirements; original candidate evidence remains unchanged.
- [x] Measure cold startup, compile, fresh evaluation, guest memory and teardown; retain reproducible commands and measurements.
- [x] Run root/nested tests, race detection, vet, formatting, bounded conversion fuzzing and CGO-free build/cross-build checks.
- [x] Request one fresh whole-change review, fix important findings with reproducing tests, and prepare review branch.

Release preparation: tag verified merged commit `c0519ea` as v1.1.0, correct the existing release target and finalize the previously approved native-library releases. These releases exclude this new prototype.
