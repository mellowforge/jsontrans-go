# JavaScript processor assessment — 2026-11-03

**Recommendation: continue with QuickJS through owned Wazero integration; do not
ship the current QJS wrapper or substitute Goja under the agreed execution profile.**
A follow-up [owned runtime checkpoint](quickjs-runtime-foundation.md) implements
the first production-plan stage and records its remaining gaps.

This assessment is implemented in an optional development module. Neither
candidate is accepted for production, and the core library has no JS dependency.

The native JSONPath/JSONata engines and public `Processor`, `Program` and registry
interfaces remain unchanged. Separately, the project minimum moves to Go 1.26
at the maintainer's request. The verified Go 1.25 baseline is tagged locally as
`v1.0.0`; [release preparation](releases-v1.md) describes the proposed `v1.1.0` update.

## Candidates and provenance

| Component | Assessment pin |
|---|---|
| Toolchain | Go 1.26.10, darwin/arm64, Apple M5 Max |
| QJS wrapper | `github.com/fastschema/qjs v0.0.6` |
| Wazero | `github.com/tetratelabs/wazero v1.9.0` |
| Goja | `v0.0.0-20261102223426-a177d211bf60` |
| Wrapper's QuickJS-ng submodule | `d01ca4491fb24ccfeccb4c7394e28a3b21fd5986` |
| Embedded Wasm | 1,038,767 bytes; SHA-256 `88fcbeead33cca3ec8394feb0fb3eec8047ec9ac5062aba7a63799cee11989b7` |

The module download checksums are committed separately. The submodule revision
comes from the wrapper tag's Git tree; this does **not** establish that the shipped
Wasm was reproducibly built from that revision. The module archive does not
include the submodule sources. Rebuilding and verifying the artifact is a
production gate. Licenses and build requirements are in the
[assessment README](../experiments/javascript/README.md).

## Gates and observed behavior

Every probe is recorded, including failures, in
[probes.json](../experiments/javascript/testdata/results/probes.json). Dangerous
probes use subprocesses with a four-second watchdog. Evaluation cancellation
uses 75 ms; source compilation cancellation starts after engine initialization.
A passing probe is evidence for that case, not a proof of arbitrary program safety.

| Required behavior | QuickJS/QJS | Goja |
|---|---|---|
| Compile once, fresh runtime, concurrent evaluations, detached input | Unit tests pass | Unit tests pass |
| Infinite-loop and conversion-proxy cancellation, fresh-runtime recovery | Pass | Pass |
| Expensive regex cancellation | Pass | **Fail:** watchdog kills child; interrupt ineffective inside regex |
| Thrown-object string coercion under cancellation | Pass | **Fail:** watchdog kills child while formatting the thrown object |
| Cancellation during source compilation | Observed guest interruption | **Fail:** compiler has no context/interrupt API; checks only surround compilation |
| Allocation growth | 128 MiB ArrayBuffer rejected by 64 MiB allocator cap | **Fail:** no per-runtime heap quota; dangerous uncapped probe not executed |
| Explicit 64 MiB guest linear-memory ceiling | **Fail:** wrapper never configures it; artifact has no memory maximum | **Fail:** no isolated guest memory |
| Cleanup after cancellation | **Fail:** `Close` panics while calling guest free with canceled context | Go-managed state; no equivalent explicit cleanup |
| Cancellation unaffected by other wrapper users | **Fail:** first cached configuration wins; watchdog kills child | No equivalent global configuration cache |
| No filesystem/network/host callback capability | **Fail by import/configuration audit:** full WASI, writable mount and Go proxy import | Bare runtime exposes none of these |
| Missing/null, finite numbers, unsupported/cyclic results, depth/items/bytes | Focused tests pass | Focused tests pass |
| Isolated UTF-16 surrogate input/output | Pass | **Fail:** JSON.parse replaces isolated input surrogate with U+FFFD |
| Default prepared-document fallback | Pass | Pass |
| Explicit unsupported instruction/per-regex controls | Rejected | Rejected |
| Customized options through registry | **Fail:** resolved native defaults conflict with strict unsupported-limit rejection | Same failure |

Non-tail recursion terminates by an engine error or deadline. The bridge rejects
functions, BigInt, Symbol, Promise/async results, nonfinite numbers, cycles,
accessors, non-plain objects, sparse arrays and extra array properties. It does
not call user `toJSON` hooks. Proxies can execute traps, so their conversion is
covered by cancellation probes. Thrown TypeError diagnostics retain native text
and stack in causes; structured JS codes/positions are not completed, and source
positions currently refer to the rule expression wrapper or the separate bridge.
Trusted bridge code is compiled independently from user source and initialized
first; regression coverage prevents crafted source replacing the converter. Error formatting
itself must also remain under cancellation; see the `exception-coercion` probe.

Numbers intentionally use binary64: `9007199254740993` becomes
`9007199254740992`. Input numeric overflow is rejected; this is distinct from
preserving JSONPath's selected-number representation. No event loop, Promise
continuation processing, runtime module loader API, Node API or host function
registration is implemented. Helpers can be prebundled inside a function expression.

The prototype uses the core JSON validators and a strict guest serializer with
work/depth/collection/output-byte checks. Input/bindings are detached. Compilation
has expression-byte and deadline checks; default steps count conversion work,
not JavaScript instructions. Explicit instruction/per-regex limits are rejected
at construction and evaluation. A production adapter needs a separate documented
JS control profile, not the native engines' full set of guarantees.

## Why the wrapper fails adoption

The [pinned runtime source](https://github.com/fastschema/qjs/blob/v0.0.6/runtime.go)
caches compiled Wasm **and** `RuntimeConfig` globally, keyed by Wasm hash, rather
than by cancellation settings. A prior default runtime disables cancellation for
later callers asking for it. The subprocess test reproduces this, without assuming
other applications will never import QJS.

`Close` calls guest cleanup using the evaluation context; after cancellation that
call panics before normal cleanup completes. It also does not explicitly close
its owning Wazero runtime or the process-global compilation runtime/cache. The
lifetime of those resources requires ownership work; these observations alone do
not quantify a retained-memory leak.

`MemoryLimit` invokes QuickJS allocator accounting, not Wazero's linear-memory
cap. The artifact declares a three-page minimum and **no maximum**. A production
runtime should additionally set `WithMemoryLimitPages(1024)` for 64 MiB, then
measure usable JS heap after stack/runtime overhead. Guest caps do not cover Go
copies, compiled code, concurrent guests or total process RSS.

The import audit finds 24 functions: a Go proxy and WASI operations including
file reads/writes, directory mutation, clocks and polling. The wrapper mounts
its CWD read/write, defaulting to the host working directory. Our prototype uses
an empty private temporary directory and discards output. That limits the test
surface; it does not establish the requested capability-free production profile
or prove that arbitrary host files are accessible from the exposed JS API.

Goja's simpler integration cannot replace the missing guarantees. Its interrupt
stops JS loops but not the tested native regex, its compiler is not interruptible,
and its heap is part of the host Go heap. Upgrading Go or setting `GOMEMLIMIT`
does not create an independent per-evaluation memory ceiling.

## Measurements

Medians of five 200 ms benchmark samples, one logical CPU, Go 1.26.10 on Apple
M5 Max/macOS arm64. Input is 100 item objects, **3,793 JSON bytes**. Compile and
runtime-init numbers below have the process-wide Wasm compilation cache warm;
source compilation is timed separately from evaluation. Each evaluation creates
fresh JS runtime state. This is a strict, unoptimized assessment bridge.

| Workload | QuickJS time | Goja time | QuickJS B/op; allocs/op | Goja B/op; allocs/op |
|---|---:|---:|---:|---:|
| Compile function + bridge | 3.218 ms | 133.0 µs | 1,521,457; 1,569 | 153,984; 2,331 |
| Fresh engine runtime initialization | 2.305 ms | 2.1 µs | 768,163; 1,384 | 5,184; 50 |
| Eval Go value → small result | 3.082 ms | 288.3 µs | 1,688,205; 2,981 | 272,153; 7,086 |
| EvalJSON → small JSON result | 3.087 ms | 582.2 µs | 1,975,533; 10,969 | 560,114; 15,075 |
| Prepared-document fallback | 2.951 ms | 376.4 µs | 1,800,141; 5,351 | 384,185; 9,457 |
| Full JSON identity round trip | 12.577 ms | 4.676 ms | 2,474,346; 22,164 | 4,522,520; 80,086 |
| Four expressions, one prepared document | 11.341 ms/batch | 1.446 ms/batch | 7,197,693; 21,271 | 1,475,879; 36,655 |
| Sixteen expressions, one prepared document | 48.062 ms/batch | 5.764 ms/batch | 28,789,188; 85,074 | 5,906,175; 146,669 |

Detached input validation + JSON encoding alone costs **90.1 µs**, 68,812 B/op,
1,196 allocations for this input. The identity round trip includes evaluation,
strict guest serialization, and host JSON decoding/encoding; it is not an isolated
JSON.stringify measurement. Prepared fallback materializes JSON for each call and
then serializes it again. It does not receive the native engines' indexed or shared
batch execution optimizations. Batch timings are per batch, not per expression.

Five fresh-process samples (cold Go/Wasm compilation caches in memory, no disk
cache): compile plus first small evaluation median **226.8 ms QuickJS**, **0.419 ms
Goja**. This timer starts inside the process and excludes OS process startup.
Maximum process RSS over those five samples: **77.89 MiB QuickJS**, **11.36 MiB
Goja**. Both measurements use the same assessment executable linking both engines.
Warmed initialization in the table above excludes the cold Wasm compilation.

The guest-memory probe observed **256 KiB** initial linear memory and about
**40.25 MiB** after a 40 MiB ArrayBuffer allocation. Its process RSS is recorded
in the probe JSON. This is a sample, not a verified cap or a maximum over all
possible workloads. Go heap and peak RSS are recorded per non-killed probe;
watchdog-killed children have no trustworthy final in-process memory observation.

Stripped `CGO_ENABLED=0`, `-trimpath -ldflags='-s -w'` macOS arm64 executables:

| Minimal executable | Bytes | Increase over bare Go executable |
|---|---:|---:|
| Bare | 1,641,362 | — |
| Goja only | 10,234,626 | 8,593,264 |
| QJS/Wazero only | 6,068,882 | 4,427,520 |
| Full assessment, both engines | 15,471,170 | 13,829,808 |

The minimal binaries estimate engine embedding cost, not a production adapter's
final size; linker reachability affects the result. Runtime builds are CGO-free
and need only Go plus cached modules. Cross-builds succeeded for linux/amd64,
linux/arm64 and windows/amd64; execution measurements cover only macOS arm64.
Rebuilding the pinned guest requires a C/WASI build toolchain, separately from
building applications that embed the artifact.

Raw [benchmarks](../experiments/javascript/testdata/results/bench.txt),
[summary](../experiments/javascript/testdata/results/summary.json),
[cold samples](../experiments/javascript/testdata/results/cold.json),
[binary sizes](../experiments/javascript/testdata/results/binary-size.json),
[import audit](../experiments/javascript/testdata/results/wasm-audit.json) and
[reproduction commands](../experiments/javascript/README.md) accompany this report.
These numbers establish a baseline, not a throughput promise or a Go 1.25/1.26 comparison.

## Production implementation plan and revised cost

Keep QuickJS/Wazero as the preferred architecture, conditional on a second
acceptance run of an owned host layer. Reject the current wrapper as-is and do
not adopt Goja with a weaker execution profile.

1. **Own the Wasm boundary and artifact (1–2 weeks).** Pin QuickJS-ng source and
   toolchain, reproduce/license the guest artifact, remove unused JS host/module
   entry points, expose a minimal versioned ABI, create instance-local immutable
   compiled-module ownership, cap guest pages, and remove filesystem mounts and
   callback registration. Fail closed on unexpected imports. Test Wazero closure,
   context interruption, canceled cleanup and conflicting configurations.
2. **Implement the optional processor module (1–2 weeks).** Keep root interfaces;
   compile function expressions with source mapping, instantiate fresh state,
   implement detached lossless UTF-16 JSON/binary64 conversion and strict result
   validation, preserve native errors and `errors.Is` cancellation. Define JS
   execution limits separately from conversion limits. Resolve registry forwarding
   of effective limits versus explicitly requested unsupported controls; this may
   need an additive public capability/options contract, reviewed before core changes.
3. **Harden and measure (1–2 weeks).** Expand compiler/built-in/error-formatting
   cancellation probes, fuzz the ABI/codec and guest resource failures, add artifact
   integrity and capability gates, stress concurrent reuse and long-running cleanup,
   run supported platform CI, benchmark cold/warm/fresh/multi-expression behavior,
   and document measured costs. Bound the conversion buffers before optimizing them.

Revised working estimate: **4–6 engineer-weeks after this assessment**, versus the
original 2–4 weeks for adapting a suitable wrapper. Source/ABI work, artifact
reproducibility and any required upstream engine patches can extend that estimate;
re-estimate if a required guarantee cannot be established. Maintaining the embedded
engine, Go/Wazero compatibility and periodic security updates is ongoing work.

No runtime pooling initially. For many expressions per document, the first
optimization candidates are bounded serialized-input reuse and reduced bridge
copies. They do not remove the cost of a fresh runtime for each independent
expression. Pooling or sessions would require a separate isolation/lifetime design.
Modules for shared code and async host-service integration remain separate future
features and are not prerequisites for the synchronous processor.
