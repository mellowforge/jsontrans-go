# JavaScript hardening record

This is the first hardening pass for the optional `javascript` module, following
module extraction. It tests the existing [execution contract](javascript-execution-contract.md);
the module remains a **preview**. It does not introduce pooling, new capabilities,
quotas, dependency updates or a release.

## Checks

Public-engine tests use test-only Wazero function listeners to observe admitted
compilation/evaluation before initiating Close. They check that concurrent Close
calls wait for complete calls, cancel them, stop admission and retain ErrClosed.
Go synctest establishes scheduler quiescence before asserting that Close remains
blocked. Removing draining is verified to fail both compilation/evaluation cases.
Separate tests observe the actual guest deadline for both raised and lowered
per-call timeout overrides. There are no sleeps used to infer guest admission.

Concurrent evaluation of one compiled program checks fresh globals, detached
input/bindings and captured bridge intrinsics after user code replaces JSON,
Reflect, String and prototype methods. Strict conversion tests cover promises,
BigInt/symbols, nonfinite numbers, boxed values, typed arrays, dates/maps,
holes/extra array properties, symbol keys, accessors and cycles. Prototype-shaped
JSON keys remain ordinary data. Output depth/item/byte rejection retains typed
phase/error information. Failed conversions are followed by healthy evaluations.
Concurrent calls receive distinct bindings and assert their individual sums;
substituting fixed bindings is verified to fail the isolation test.

JSON-tree fuzzing compares bounded, normalized JSON values against Go's binary64
JSON model, including nested objects/arrays and prototype-shaped keys. Existing
compiler, string, diagnostic-protocol and location fuzzers remain in place.
Normalization intentionally excludes decimal-spelling/surrogate-spelling
comparisons; dedicated compatibility tests cover those semantics.

The public-only `javascript/cmd/hardening` tool mixes successful calls, throws,
cyclic results, deadline cancellation and a 40 MiB allocation under an 8 MiB guest
cap. It verifies the exact expected outcome and a fresh healthy call after every
iteration, then closes/reconstructs engines across cycles. OOM tests reject
deadline/cancellation/closure causes rather than treating any error as success.
The command bounds workers, iterations, cycles, total work and overall duration,
propagates worker failure, and reports observations only after a successful run.
Its cancellation workload exercises cancellation under contention; the separate
admission/assessment tests establish in-guest interruption semantics.

Linux CI adds bounded JSON-tree fuzzing and a short soak to the existing complete
test/race/vet, packaging and 21-probe gates. A separate Windows job runs module
tests, race detection and vet. Existing native/macOS jobs are unchanged. Local
measurements below are on macOS; CI results, rather than cross-compilation alone,
are required before describing another platform as verified.

## Measurements

Environment: Apple M5 Max, darwin/arm64, Go 1.26.10, CGO disabled for standalone
tools; benchmark default GOMAXPROCS=18. These are small-fixture development
measurements, not throughput guarantees. Public Eval always creates a fresh guest.
The fixture has two items priced 7 and 14; the expression returns their total.
Compiled expressions and prepared documents are created outside evaluation timers.

Median of three benchmark samples:

| Public operation | Time/op | Go B/op | Go allocations/op |
|---|---:|---:|---:|
| NewClose | 186.245 ms | 45,023,124 | 76,740 |
| Compile | 2.537 ms | 4,519,985 | 1,146 |
| Eval | 0.845 ms | 2,293,292 | 675 |
| EvalJSON | 0.846 ms | 2,300,390 | 805 |
| PreparedFallback | 0.850 ms | 2,297,244 | 736 |
| PreparedBatch4 | 3.360 ms | 9,186,721 | 2,909 |

The final soak checked 40,000 mixed iterations plus 40,000 recovery
evaluations and 10 first evaluations across ten engine lifecycles, four workers,
in 63.04 seconds. Each outcome category occurred 8,000 times.
Median construction was 189.00 ms and the median first constant-expression
Eval was 1.101 ms. Post-close/post-GC samples ranged from
0.38 to 1.00 MiB retained Go heap; each sample had one goroutine.
Maximum process RSS was 79,937,536 bytes (76.23 MiB), separately measured by macOS time.
The soak deliberately uses an 8 MiB cap; benchmark engines use the 64 MiB default.

Stripped, CGO-free darwin/arm64 binaries were 1,641,362 bytes for the trivial
Go CLI and 6,289,730 bytes for the public-adapter example: a
4,648,368-byte increase (4.43 MiB). This includes the adapter and its
required core/runtime code, rather than isolating the embedded guest alone.


Raw [benchmark output](../javascript/testdata/hardening/bench-darwin-arm64.txt),
[soak JSON](../javascript/testdata/hardening/soak-darwin-arm64.json),
[process resource usage](../javascript/testdata/hardening/soak-darwin-arm64.time.txt)
and [owned probes](../javascript/testdata/hardening/owned-probes.json) are retained.
Construction is a fresh engine/cache in an already running process, not a
process-launch measurement. First Eval uses a compiled constant expression; the
steady-state benchmarks use the two-item fixture. They are different workloads.
Compilation in the soak covers six expressions, including the recovery program.
Go B/op and allocs/op exclude executable mappings and are not peak RSS. Post-GC
heap/goroutine samples and a finite soak cannot establish absence of leaks.

Reproduce from the repository root (full history and cached dependencies are
required by the existing packaging checker):

```sh
go -C javascript test -run '^$' -bench '^BenchmarkJavaScript$' -benchmem -benchtime=300ms -count=3
CGO_ENABLED=0 go -C javascript build -o /tmp/jsontrans-js-hardening ./cmd/hardening
# macOS reports maximum RSS in bytes; Linux time -v reports KiB.
/usr/bin/time -l /tmp/jsontrans-js-hardening -iterations 1000 -workers 4 -cycles 10
# Linux equivalent: /usr/bin/time -v /tmp/jsontrans-js-hardening ...
CGO_ENABLED=0 go -C experiments/javascript build -trimpath -ldflags='-s -w' -o /tmp/jsontrans-js-bare ./cmd/bare
CGO_ENABLED=0 go -C experiments/javascript build -trimpath -ldflags='-s -w' -o /tmp/jsontrans-js-owned-min ./cmd/owned-min
go -C javascript test -run '^$' -fuzz '^FuzzOwnedJSONConversion$' -fuzztime=500x -parallel=2
```

Guest ABI/source/toolchain pins and the 929,138-byte artifact remain unchanged;
SHA-256 `29556e8cbe4b78704c71304aa3da83ae8881dd3d1447c65d27ade3237721d60e`.
Production applications need Go only, with no C compiler/Node runtime; rebuilding
the guest requires Python and the pinned WASI SDK. No dependency upgrade was
performed in this pass.

## Operational and release gates

Keep engine admission/concurrency bounded at the application boundary. A 64 MiB
default guest cap applies per guest and excludes Go allocations, compiled code
and simultaneous calls. Per-call timeout overrides are application-controlled;
they are not a ceiling on untrusted options. Close cancels/drains calls but is not
a hard shutdown deadline or a physical-memory reclamation guarantee.

The host instantiates no WASI/host modules, mounts no filesystem and exposes no
network/callback/import service. The embedded binary is hash checked and its
imports/exports/memory declarations are audited at construction. Only private
package-generated bytecode is accepted; no bytecode-loading API is exposed.
Diagnostics can contain user data, control characters and forged frames; apply
application logging/redaction/rendering policy. Reported locations are not proof
of authenticated execution positions.

Before a production release, complete a current Go dependency vulnerability scan
and manual QuickJS/WASI/Wazero security-notice review, prolonged soaks on intended
deployment platforms, broader ABI/resource-failure fuzz campaigns and review of
the resulting CI/measurement evidence. Repeat on pin/toolchain changes. Guest
updates must carry source/SDK hashes and licenses, reproduce byte-for-byte, pass
capability/integrity checks, all 21 probes and the full race/fuzz/stress/platform
gates. No automated Go scanner covers the embedded C engine by itself.

Choose a monitored private reporting route before public release, then separately
approve the supported execution profile, platform policy and module tag. This
pass provides repeatable gates and a measured baseline; it does not approve a
stable release or assert hard in-process resource isolation.
