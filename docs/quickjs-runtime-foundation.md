# Owned QuickJS/Wazero checkpoint — 2026-11-03

At this checkpoint, the experimental owned runtime passed **17 of 18** recorded assessment probes,
including all ten selected resource/capability gates. The one remaining probe
failure was registry limit forwarding. This is a successful runtime checkpoint,
**not a production JavaScript processor or a claim of complete JavaScript conformance**.

The [registry follow-up](registry-limit-forwarding.md) resolves that failure:
all **18 of 18** recorded probes now pass. The measurements and original probe
results below describe the earlier checkpoint and remain unchanged.

The native library's [v1.1.0 release](https://github.com/mellowforge/jsontrans-go/releases/tag/v1.1.0)
is tagged at verified merged commit `c0519ea48d8022721b0d847e3def176d5a99e361`.
This new prototype is subsequent work and is not included in that release.

The subsequent [execution contract](javascript-execution-contract.md) adds
owned lifecycle/typed diagnostics and ABI v2. Artifact/measurement numbers below
remain the historical ABI v1 checkpoint.

## What changed

`experiments/javascript/internal/quickjswasm` owns its Go/Wazero boundary and a
small C ABI linked only to QuickJS-ng core. It does not call the QJS wrapper.
The original QJS and Goja assessment backends and observations remain available.
The optional `owned-quickjs` backend reuses the independently compiled strict
conversion bridge and existing public root interfaces. Root Go dependencies,
JSONPath and JSONata implementations are unchanged.

- The guest has **zero host imports**. No WASI instance, filesystem mount,
  network API, output stream, module loader, Go callback or async job runner is
  registered. Latent libc clock/output/exit operations resolve to guest-local
  snapshots, denied operations or traps; they acquire no host capability.
- The Wasm artifact declares a **64 MiB maximum**, and each owned Wazero runtime
  additionally configures an explicit page ceiling. Tests exercise independent
  8 MiB and 64 MiB engines concurrently, failed growth beyond the page maximum,
  and rejection of a 40 MiB buffer by the smaller engine.
- Context termination is always enabled. Cancellation stops source compilation,
  loops, regex execution, conversion proxy traps and thrown-object coercion.
  Fresh evaluations recover after cancellation and allocation failures.
- Each compile/evaluation instantiates fresh guest state. Compiled rule and bridge
  bytes are immutable Go-owned copies; the trusted bridge loads first, separately
  from user source. Inputs and bindings retain the existing detached JSON boundary.
- Canceled or trapped modules are discarded through Wazero with a background
  cleanup context. Cleanup never attempts guest frees using a canceled context.
  `Engine.Close` cancels active operations and closes its runtime and compilation
  cache. Programs cannot be used after their engine closes.

There is no global configuration or compilation cache in the owned runtime.
A probe initializes the unrelated QJS wrapper with cancellation disabled and
confirms that the owned engine still cancels and recovers.

## Artifact and build provenance

| Item | Pin |
|---|---|
| QuickJS-ng source | `d01ca4491fb24ccfeccb4c7394e28a3b21fd5986` |
| Source archive SHA-256 | `40be727abd6f7d24911b6a33fc3d25603a3f89eee183f74ffe6ca8d5619d9922` |
| WASI SDK | 34.0, checksummed macOS arm64 and Linux x86_64 assets |
| Wazero | 1.9.0 |
| ABI version | 1 |
| Artifact | 923,613 bytes |
| Artifact SHA-256 | `16fb2a9c4233a4d216e103f86ba25f53516ee1b8e358e4a41cb62056f08ae4ae` |

Two clean builds in separate temporary directories on macOS arm64 produced
identical bytes. A Linux CI job rebuilds with its separately pinned SDK asset
and compares against the committed artifact; cross-host reproducibility remains
pending that job until it passes. The source archive, SDK component revisions,
licenses and artifact fingerprint are retained under
[`guest`](../experiments/javascript/internal/quickjswasm/guest).

Applications use the embedded artifact and build with **CGO_ENABLED=0**, without
a C compiler or Node. Rebuilding the artifact needs Python 3.12+, the downloaded
SDK's clang/linker and the pinned source archive; it does not need CMake or
Binaryen. Downloads are verified before extraction/execution. The build checks
in no compiler or upstream source tree. The QuickJS MIT, wasi-libc/musl/cloudlibc
and LLVM compiler-runtime notices accompany the artifact.

## Explicit prototype profile

The following choices keep this checkpoint small and must be reviewed before
production:

- An evaluation receives a **UTC time snapshot**, also used to initialize that
  guest's local Math.random state. Date.now does not advance during an evaluation;
  this is not a live clock or a cryptographic RNG guarantee.
- Wasm/C stack reservation is fixed at 1 MiB. QuickJS-ng disables its native
  stack-byte checks under WASI; explicit `StackBytes` / `GuestStackBytes` settings
  are therefore rejected as unsupported. No JS recursion/stack quota is promised.
  Recursion can trap within the isolated guest, which is discarded; this is not
  a native JS stack-overflow guarantee. The guest starts with 2 MiB linear memory;
  configurable guest caps are 2–64 MiB in 64 KiB pages. Conversion tree depth is
  bounded separately. Verified JS stack accounting remains future work.
- ABI transfers are capped at 16 MiB each; compiled bytecode at 4 MiB; exception
  text at 64 KiB. Explicit incompatible limits fail. The bytecode interface is
  internal and accepts only locally compiled artifacts, never user bytecode.
- Default timeouts remain one second. Trusted semantic/race fixtures raise them
  to ten seconds in ordinary tests and 100 seconds in race builds to accommodate
  instrumented Wasm compilation. Explicit cancellation deadlines stay unchanged. Cold compilation
  of the fixed trusted Wasm artifact is distinct from user-source compilation
  inside the interruptible guest; initialization is not a hard process deadline.
- JS instruction counting and independent per-regex deadlines are not promised.
  Conversion steps/depth/items/bytes remain bounded. Unsupported explicit step
  and regex settings are rejected, including the currently over-forwarded settings.
- Native exception messages are retained; complete typed JS diagnostic codes,
  source positions and source mapping remain production work.

The 64 MiB ceiling bounds **guest linear memory**, including ABI buffers and the
C stack. It does not bound Go copies, JIT code, Wazero call stacks, aggregate
concurrent guests or process RSS. Module Close measurements describe logical
teardown; Go garbage collection can reclaim backing storage later.

## Measurements

Go 1.26.10, Apple M5 Max, macOS arm64, one logical CPU; medians of five 200 ms
samples unless stated otherwise. Input: 100 item objects, 3,793 JSON bytes.
The owned runtime's Wasm compilation cache is warm; every evaluation creates a
fresh guest. This includes strict conversion overhead, not only JS expression execution.

| Workload | Time | B/op | Allocations/op |
|---|---:|---:|---:|
| Compile rule + bridge | 2.043 ms | 4,510,141 | 1,136 |
| Initialize and close a fresh guest | 594.8 µs | 2,164,948 | 522 |
| Eval Go value, small result | 1.122 ms | 2,360,805 | 1,827 |
| EvalJSON, small result | 1.399 ms | 2,648,767 | 9,816 |
| Prepared-document fallback | 1.221 ms | 2,472,839 | 4,198 |
| Full JSON identity round trip | 10.353 ms | 3,151,711 | 21,012 |
| Four expressions, one prepared document | 4.406 ms/batch | 9,890,612 | 16,660 |
| Sixteen expressions, one prepared document | 17.665 ms/batch | 39,560,703 | 66,627 |

Guest Close alone measured approximately 0.1 µs, zero Go allocations, using five
samples of **100 fixed iterations** with initialization excluded. This excludes
later garbage collection and is not a physical-memory reclamation guarantee.

Five fresh-process compile + first-evaluation samples measured **189.6 ms**
median and **57.94 MiB** maximum peak RSS. The timer starts inside the process,
excluding OS process startup. The 40 MiB ArrayBuffer probe observed 2 MiB initial
and about 41.19 MiB final linear memory, with 99.02 MiB process peak RSS.

A stripped minimal owned-engine executable is **6,103,746 bytes**, built with
CGO disabled, `-trimpath -ldflags='-s -w'`. The all-three-backend assessment
executable is 16,412,898 bytes and is not the size of a future production adapter.
CGO-free cross-builds passed for Linux amd64/arm64 and Windows amd64; execution
measurements cover only macOS arm64.

Raw observations, build evidence and benchmark summaries are in
[`testdata/results/owned`](../experiments/javascript/testdata/results/owned).
These are baseline measurements, not a throughput promise. The larger minimum
linear memory contributes to Go B/op; the library does not claim zero allocations.

## Remaining work and next decision

The registry follow-up resolves original versus synthesized limits through an
additive capability contract. CI now requires all 18 owned probes to pass;
historical results retain the original forwarding failure for provenance.

The next stage is the production adapter design: choose clock
semantics, complete structured diagnostics and source mapping, and define engine
ownership/Close behavior for callers. Then implement the optional public module,
fuzz/stress it further, audit/update the embedded engine before production, and
rerun the complete acceptance matrix. No async, imports, worker processes,
callbacks or pooling are introduced by this checkpoint.
