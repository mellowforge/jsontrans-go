# Owned QuickJS upgrade and terminal failures

PR B upgrades the optional preview module after merged PR #12
(`3fc979a0d97d93cde0b92f6bb6dc7a6bdf244c2a`). The native JSONPath/JSONata engines,
root contracts, Go dependencies and assessment-only QJS baseline remain unchanged.
This record supersedes the old-guest status in the historical security audit;
it does not rewrite that audit or approve a production release.

## Source and artifact

| Component | Pin |
|---|---|
| QuickJS-ng | v0.18.0, `d7215b0ad3fc2c562e8317fd9bca058fd4863804` |
| Source archive SHA-256 | `a49defe70cbcae9226d9fac025af116e87ebae2440dcff1ca3c80b79e1cdef0b` |
| Owned OOM patch SHA-256 | `41e04680ad186ec1d230874e949593bf0723c443c31966941509e25461eb146b` |
| WASI SDK | 34.0; per-host asset checksums and LLVM/libc revisions unchanged |
| Guest ABI / diagnostic JSON | 3 / 2 |
| Owned Wasm | 1,034,164 bytes |
| Wasm SHA-256 | `31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee` |

The [manifest](../javascript/internal/quickjswasm/guest/lock.json) pins the
immutable upstream archive and the separately hashed owned patch. The build
verifies both and applies the patch with zero fuzz. Source archive cache names
include the commit, preventing an old cached archive from shadowing a new pin.
Upstream v0.18 moved cutils into a header; the owned build follows its core
source list. No upstream reactor, JS libc wrapper or prebuilt guest is adopted.
Updated upstream MIT notices are retained.

## Terminal-error behavior

OOM and InternalError stop an evaluation, including failures during source
initialization, rule execution, output traversal and diagnostic getters. A catch
block cannot resume the evaluation; no result is returned. The next call uses a
fresh guest. Ordinary exceptions and trusted conversion/limit failures retain
their existing behavior.

Native exception fields are preserved when available, otherwise a bounded
fallback is returned as a typed Exception. Names, messages, stack text and the
native uncatchability flag do not grant ErrLimit or ErrConversion provenance.
Wasm traps remain distinct causes, rather than being relabeled by their text.
Compiler columns now retain the pin's native values, calibrated for ASCII,
Unicode and CRLF; byte offsets remain unavailable and runtime locations remain
explicitly reported/untrusted.

### Owned complete-exhaustion patch

Upstream v0.18 makes native OOM/InternalError uncatchable, but `JS_ThrowError2`
can replace an allocation failure with catchable JS_NULL if constructing the
error object fails. A tight linked-node reproduction caught that value and
continued beyond OOM. Merely changing the reported phase would hide the problem.

With explicit approval, the [owned patch](../javascript/internal/quickjswasm/guest/quickjs-terminal-oom.patch)
reserves an uncatchable OOM error in each full JSContext, after base intrinsics
and before the remaining context initialization/user code. It explicitly marks
the GC reference and frees it with the context. JS_ThrowOutOfMemory and failed
error construction duplicate the reserved value without allocating. Before
reserve initialization, upstream failure behavior remains available to context
construction; failed construction never enters user evaluation. Raw contexts
are not exposed by the owned ABI, which always uses JS_NewContext.

This is an owned variant of v0.18, not a claim that upstream includes the fix.
The final review also found partial error construction and stack formatting
could overwrite the reserved error with an ordinary catchable exception. The
patch propagates failed message/property creation, preserves terminal errors
when restoring a saved exception, and propagates stack-formatting failure
through constructors and captureStackTrace. It avoids invoking a formatter
after an earlier terminal stack-limit conversion/allocation failure. Ordinary
formatter exceptions keep upstream behavior. Partially constructed objects are
freed. Public regression cases failed before this correction and pass with it;
a native 513-limit sweep checks partial error construction without source
instrumentation. The supported 2 MiB minimum has a retained recovery regression.
The patch must be reviewed/rebased on every source update. A native development
harness exercises repeated context collection and partial initialization under
ASan, UBSan, leak checking and QuickJS leak assertions. Omitting the added GC
mark makes that harness reject retained objects, validating the lifetime check.
The native harness is never linked into the production Wasm or Go runtime.

### Private phase transport

C preallocates a null-prototype record with an own writable integer data slot
before user source initialization. Only C and the trusted strict bridge retain
it; rule inputs and bindings do not contain it. The slot starts at evaluate=0,
and the bridge writes output=1 before traversing the result. Existing-slot
updates require no allocation. On an exceptional C return, its own descriptor
is validated without getters, atom allocation or value coercion. Invalid private
records fail internally rather than supplying invented native provenance.

The internal ABI is versioned to 3. Export signatures and Go interfaces remain
unchanged; diagnostic JSON retains version 2. Compiled programs are local to
their engine/artifact; there is no external bytecode API or runtime pooling.

## Limits and capability boundary

Defaults and supported ranges remain unchanged: a 1-second call timeout,
64 MiB guest cap configurable from 2–64 MiB in 64 KiB pages, a 1 MiB reserved C
stack, 16 MiB transfers and 4 MiB bytecode. Conversion budgets remain bounded;
JS instruction, separate regex and stack quotas remain unsupported. No host
imports of any kind, WASI instance, filesystem/network, entropy service, Go
callback, async job pumping or dynamic runtime loader is enabled. The UTC time
snapshot and guest-local RNG profile remain unchanged.

## Verification and release status

Focused regressions cover the original oversized Proxy key-list trap with
small/hole/duplicate/symbol/getter controls, terminal failures in each phase,
tight-heap continuation, diagnostic OOM, prototype mutation, terminal-error
forgery and fresh recovery. Old catch-OOM-and-continue expectations are replaced
explicitly. Separate nonterminal-pressure tests preserve trusted conversion and
limit classification plus ordinary Proxy exceptions.

The native OOM/GC harness passed on Bertram, including twelve context cycles
and 61 initialization limits. Its missing-GC-mark mutation failed as expected.
The terminal-phase fuzz seeds also reject an omitted bridge phase update.
The [dated evidence](../javascript/testdata/security/2026-11-04-quickjs/post-review/README.md)
records native Mac/Linux tests, race detection and vet in all three modules,
both native Go versions, the Node oracle, all ten bounded fuzz targets,
outside-consumer packaging and all 21 owned probes. Native Windows x64 public
module tests, race detection, vet and the CGO-free example also pass. Windows
uses private checksum-checked Go 1.26.10 and MinGW-w64 14 toolchains. Linux and
Mac SDK 34.0 rebuilds produce the identical recorded guest. No hosted Actions
runs are required; these local results do not change GitHub check statuses.

The Linux deployment-host soak verifies 100,000 independent failure/success and
healthy-recovery pairs over five engine lifecycles. Post-GC goroutine counts
return to one. This bounded run supplements, rather than replaces, prolonged
deployment testing.

The Go source/binary security matrix passes for Linux amd64, Windows amd64 and
Mac arm64. A review of all ten currently published upstream advisories checks
the pinned C source, not just advisory ranges; see the evidence's source review.

Go scans cannot inspect embedded C code. Review current upstream notices and
the actual reachable source fixes rather than trusting advisory version ranges
alone. Production approval remains separate: review the terminal contract,
platform and soak evidence, and confirm a monitored private reporting route.
No module tag or release is created by this PR.

### Measured upgrade cost

Matched runs on Apple M5 Max / darwin arm64 / Go 1.26.10, GOMAXPROCS=1,
five samples at 200 ms per benchmark. Values are medians; small differences
are descriptive and have no statistical significance claim. The old version is
merged PR #12 with the old owned guest, retained only as a measurement baseline.
Eval includes a fresh guest; the prepared API uses the JSON fallback.

| Operation | Before | After | After bytes/op | After allocations/op |
|---|---:|---:|---:|---:|
| New + Close | 197.93 ms | 230.77 ms | 49,076,032 | 101,631 |
| Compile | 2.466 ms | 2.492 ms | 4,545,843 | 3,301 |
| Eval | 759.3 µs | 739.6 µs | 2,302,087 | 1,749 |
| EvalJSON | 763.6 µs | 743.8 µs | 2,309,198 | 1,880 |
| Prepared fallback | 750.2 µs | 759.3 µs | 2,306,071 | 1,811 |
| Prepared batch, four expressions | 3.000 ms | 2.958 ms | 9,222,025 | 7,206 |

Five independent cold processes have median construction/compile/first-evaluation
latency 191.85 → 231.00 ms and maximum sampled process RSS 58,900,480 →
62,210,048 bytes. The CGO-free basic application grows from 9,153,090 to
9,252,162 bytes; Wasm grows from 929,138 to 1,034,164 bytes. Binary size includes
Go metadata. Guest initial/max linear memory remains 2/64 MiB; cap tests still
accept a 40 MiB buffer with the 64 MiB configuration and reject it at 8 MiB.
Process RSS and Go allocations include runtime/compiler state and are not guest
heap measurements or hard total-process memory limits.

The clearest cost is cold engine construction (about 17% in New + Close and
20% in the cold probe). Reused-engine calls keep roughly the same latency scale,
with 147 additional allocations per Eval in this workload. These measurements
cover the combined source upgrade and owned phase/OOM changes; they do not
isolate the cost of the OOM reserve. This PR prioritizes terminal behavior and
the unsafe-source replacement, with no pooling or performance promise.

## Reproduce

From the checkout root, use the [local verification guide](local-verification.md).
The focused checks are:

```sh
go -C javascript test -run 'TestOwnedTerminal|TestOwnedProxyKeyListSafety|TestOwnedPrivatePhase' -count=1
go -C javascript test -run '^$' -fuzz '^FuzzOwnedTerminalPhases$' -fuzztime=200x -parallel=2
python3 javascript/internal/quickjswasm/guest/build.py --output /tmp/quickjs-pr-b.wasm
cmp javascript/internal/quickjswasm/quickjs.wasm /tmp/quickjs-pr-b.wasm
```

For the native lifetime check on Linux, extract the checksum-pinned source into
a separate temporary directory, apply the recorded patch with `patch -p1 -F0`,
then build its core sources and the repository's
`javascript/internal/quickjswasm/guest/test_terminal_oom.c` using:

```sh
gcc -O1 -g -D_GNU_SOURCE -DENABLE_DUMPS -fno-omit-frame-pointer \
  -fsanitize=address,undefined -I. dtoa.c libregexp.c libunicode.c quickjs.c \
  /absolute/path/to/test_terminal_oom.c -lm -pthread -o test-terminal-oom
ASAN_OPTIONS=detect_leaks=1:abort_on_error=1 UBSAN_OPTIONS=halt_on_error=1 ./test-terminal-oom
```

Success prints `terminal OOM, GC roots and context-failure cleanup: PASS`.
This check needs a native C compiler solely for development verification;
applications continue to build and run the public processor with CGO disabled.
