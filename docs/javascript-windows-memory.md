# Windows post-Close memory investigation

The intermittent post-Close Go-heap spikes recorded in PR #14 are explained by
Wazero's asynchronous cancellation-monitor shutdown. The monitors briefly retain
closed Wasm instances and their module data while they await scheduling. They can
be present when GC scans its roots and finish before the subsequent goroutine
count is read. The retained objects then become collectible in a later cycle.
This investigation reproduced that mechanism on native Windows and removed it
with a diagnostic-only control. It did not find cumulative retention in these
runs or change the production runtime.

## Evidence and controls

Base source is merged main `8e7e84c9e53d89923658da9139d797575ed745fe`, Go
1.26.10, Wazero v1.12.0, Windows/amd64. The diagnostic test executables were
cross-compiled on macOS/arm64 with CGO disabled, then executed on the Windows VM.
The guest artifact and production dependencies remain unchanged. Each lifecycle
uses a 64 MiB guest cap and the existing hardening workload: 25 iterations on
each of two workers, covering success, exception, conversion, cancellation and
allocation failure, each followed by a checked healthy evaluation.

The [archived evidence](../javascript/testdata/memory/2026-11-04-windows/README.md)
contains exact sources, runner scripts, binary identities, samples, profiles,
stack traces and checksums. The 1 MiB threshold below identifies large residual
snapshots for this experiment; it is not a supported API memory bound.

| Diagnostic | Lifecycles | Checked recoveries | First-GC samples over 1 MiB | Largest first-GC heap | Largest second-GC heap |
|---|---:|---:|---:|---:|---:|
| Original runtime, profiling enabled | 40 | 2,000 | 20 | 6,078,552 B | 316,904 B |
| Original runtime, profiling disabled | 80 | 4,000 | 30 | 6,085,312 B | 300,848 B |
| Diagnostic synchronous-monitor control | 80 | 4,000 | 0 | 230,088 B | 229,760 B |

Every second- and third-GC snapshot in all three runs was below 317 KiB. In the
80-cycle unmodified run, all 30 large first-GC samples began with pending extra
goroutines. The 45 cycles already at the two-goroutine test baseline had no
spike. Five other cycles had pending goroutines but no spike, consistent with
the scheduler being able to drain them before marking. The synchronous control
had only the two baseline goroutines before GC in all 80 cycles. The original
campaign's baseline of three also includes its signal-handling machinery.

Twenty sampled heap profiles identify `wasm.NewMemoryInstance`, decoded Wasm
code and module/function metadata. Profiles locate allocations, not retaining
roots, and Go's heap profiler may lag collection; they are supporting evidence,
not a stand-alone proof. A separate ten-snapshot diagnostic with GOMAXPROCS=1
captured 11–27 runnable
`ModuleInstance.CloseModuleOnCanceledOrTimeout.gowrap1` monitors immediately
after Close. The GOMAXPROCS=1 run amplifies scheduling effects and is not the
80-cycle comparison, which uses GOMAXPROCS=2 throughout.

## Mechanism

Wazero's [call engine](https://github.com/tetratelabs/wazero/blob/v1.12.0/internal/engine/wazevo/call_engine.go)
starts a cancellation monitor for each exported Wasm function call when
`WithCloseOnContextDone(true)` is enabled. That monitor receives the module
instance. Its [returned cancellation function](https://github.com/tetratelabs/wazero/blob/v1.12.0/internal/wasm/module_instance.go)
closes a channel to request termination; it does not join the goroutine.

The public adapter drains its admitted operations before closing the runtime
and compilation cache. That does not synchronously join Wazero's internal
monitors. A runnable monitor can still hold its module instance, which references
the linear-memory buffer and module/compiled metadata. If GC marks those roots
before the monitor exits, those objects survive that cycle even though the
monitor is gone by the time `NumGoroutine` is sampled. The next collection can
reclaim them. This explains why the original report could show three goroutines
and several MiB of residual heap in the same post-GC snapshot.

To test causality, a temporary dependency copy changed only the monitor helper:
it closes a completion channel on goroutine exit, and its returned cancellation
function waits for that channel after requesting termination. The same
80-lifecycle test then had no pending monitors or large residual snapshots.
The [patch](../javascript/testdata/memory/2026-11-04-windows/sources/wazero-monitor-wait.patch)
is an experiment, not an adopted dependency fork. No production module file or
runtime code uses it.

## Recommendation and limits

Keep the production cancellation controls and Close behavior. Do not add sleeps,
forced GC, or a per-Wasm-call synchronization patch to library evaluation to
improve a measurement. The diagnostic patch changes every exported function
call and would need a separate performance and cancellation assessment before
adoption; these short runs are not throughput benchmarks.

Keep the original single-GC samples intact. When investigating suspicious
closed-state samples, record the pre-GC goroutine count and separate first,
second and third collection observations as done here. Do not silently replace
an initial high sample with a settled value. Neither a second GC nor a
particular goroutine count is a general API guarantee of reclamation.

The mechanism is not specific to Windows; its occurrence at the sampling
boundary depends on scheduling and workload. This investigation establishes the
cause of the reproduced Windows pattern, not a proof against every leak or an
explanation of all possible process working-set growth. Guest memory limits do
not cap total process memory, and Close does not promise immediate Go-heap or
physical-memory reclamation. Production/platform approval and release remain
separate decisions.
