# Optional JavaScript processor

A CGO-free synchronous JavaScript processor for jsontrans-go, powered by an owned
QuickJS-ng/WebAssembly guest and Wazero. **v1.0.0 is approved for production**
under the documented [execution profile](../docs/javascript-execution-contract.md)
and [platform policy](../docs/javascript-production-readiness.md). The native
JSONPath/JSONata module has no JavaScript dependency.

```go
import (
    "context"
    jt "github.com/mellowforge/jsontrans-go"
    js "github.com/mellowforge/jsontrans-go/javascript"
)

engine, err := js.New(ctx, js.Config{})
if err != nil { return err }
defer engine.Close()
transformer, err := jt.New(map[string]jt.Processor{"js": engine})
if err != nil { return err }
result, err := transformer.ProcessJSON(ctx, "js", []byte(`{"price":7}`),
    `(input, bindings) => ({total: input.price * bindings.quantity})`,
    jt.EvalOptions{Bindings: map[string]any{"quantity": 3}},
)
// result.Value is {"total":21}; handle err before using it.
```

`New(ctx, Config) (*Engine, error)` uses ctx for initialization only. The caller
owns the engine; the registry borrows it. Compile once and evaluate concurrently
with independent input/bindings. Each evaluation creates fresh guest state.
Close stops admission, cancels and drains complete calls, then closes runtime
and cache resources. Programs cannot be used after closure; errors.Is finds
ErrClosed and context cancellation through registry/JSON/prepared/batch helpers.
The root `EvalPreparedJSON` and `Batch.EvalPreparedJSON` helpers also work with
compiled JavaScript programs, through detached materialization and the same
fresh-runtime profile. They require a core version providing those APIs; the
adapter's existing minimum dependency does not include them. See
[prepared JSON results](../docs/prepared-json-results.md).

```go
var exception *js.Exception
if errors.As(err, &exception) {
    // Native Name/Message/Stack and optional SourceLocation; bounded text.
}
```

JSON input/bindings are detached; numbers become binary64. Top-level undefined
is missing; null stays defined. Strict result conversion rejects promises,
functions, BigInt, symbols, cycles, nonfinite numbers and unsupported object
shapes. Source initializers run during Eval, not Compile. No async jobs, imports,
Node APIs, filesystem/network APIs, host functions or runtime pooling are enabled.

The existing restricted execution contract is preserved: a UTC time snapshot
per guest, noncryptographic guest-local RNG, cooperative Go conversion limits,
wall-clock cancellation and a default 64 MiB guest linear-memory cap. Configured
memory ranges from 2–64 MiB in 64 KiB pages. Explicit JS instruction counting,
separate regex deadlines and stack quotas remain unsupported. These controls do
not bound total process RSS. Native columns are retained, while byte offsets
remain unavailable; runtime locations are explicitly reported/untrusted.

OOM and InternalError stop the evaluation, including when they occur during
output conversion or diagnostic formatting. JavaScript catch blocks cannot
resume after them. Errors retain native fields or a bounded fallback, and the
next evaluation starts with a fresh guest. Native error names/messages never
grant trusted ErrLimit/ErrConversion classification.

## Installation and versions

Module path: `github.com/mellowforge/jsontrans-go/javascript`; package: javascript.
Install the separately versioned public module:

```sh
go get github.com/mellowforge/jsontrans-go/javascript@v1.0.0
```

The first supported version is **v1.0.0**, tagged `javascript/v1.0.0` in this
repository. JavaScript tags use the nested module prefix `javascript/vX.Y.Z`;
root-library tags use `vX.Y.Z`. The minimum
root dependency is the real merged version
`v1.1.1-0.20261103173822-e9b9c0c71fe0`, containing registry limit forwarding;
released core v1.1.0 is too old for this adapter. Go 1.26 or newer is required.

The [historical dependency/security review](../docs/javascript-security-review.md)
identified unsafe patterns and a reproducible trap in the old guest. The
[QuickJS upgrade record](../docs/javascript-quickjs-upgrade.md) describes v0.18.0,
the owned terminal-OOM patch and the updated verification gates. Go dependency
updates and the automated scan gate are also applied. The
[approval record](../docs/javascript-production-readiness.md) and
[v1.0.0 release notes](../docs/releases/javascript-v1.0.0.md) record the accepted
profile and verification evidence.

The local `replace github.com/mellowforge/jsontrans-go => ..` is a development
convenience only. Consumers ignore it and resolve the real minimum version.
The module reuses the repository's root internal budget/JSON utilities through
its shared import-path prefix; packaging checks exercise that from outside the
checkout. Core/helper changes must continue to be tested with this submodule.

## Development and verification

From this directory:

```sh
go test ./...
go test -race ./...
go vet ./...
python3 scripts/test_check_module.py
python3 scripts/check_module.py
go test -run '^$' -fuzz '^FuzzOwnedStringConversion$' -fuzztime=200x -parallel=2
go test ./internal/quickjswasm -run '^$' -fuzz '^FuzzOwnedCompile$' -fuzztime=200x -parallel=2
```

Run tests first to cache normal dependencies. The packaging checker builds a
module-zip fixture, uses a private Git mirror advertising the pinned commit, then Go's normal
local-Git fetch to validate the minimum
core version, then builds/runs an outside consumer with no replace/workspace,
a fresh module cache and CGO disabled. It runs the executable from an empty
working directory and checks dependency isolation. It requires full Git history
and cached dependencies, and needs no network or credentials during the check.

The canonical Wasm/ABI sources, manifest and third-party notices live under
`internal/quickjswasm/guest`. The current artifact hash is:
`31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee`
(1,034,164 bytes, ABI 3, zero host imports). Applications use the embedded artifact
without a C compiler or Node; rebuilding uses the pinned WASI SDK, Python and
the standard `patch` utility. Source archive and owned patch checksums are
validated separately; patch application permits no fuzz:

```sh
python3 internal/quickjswasm/guest/build.py --output /tmp/quickjs.wasm
cmp internal/quickjswasm/quickjs.wasm /tmp/quickjs.wasm
```

The assessment module under `../experiments/javascript` keeps the historical
QJS/Goja comparison tools and forwards owned calls here. Its strict runner still
requires all 21 owned cases to pass. Internal page-growth/statistics tests moved
with this runtime; the assessment facade checks large/small public memory caps.
See the repository's [execution contract](../docs/javascript-execution-contract.md)
and [processor atlas](../docs/processor-architecture.md). Module archives retain
this README and the guest notices; repository navigation links are intended for
source checkouts.

## Hardening and measurements

The [hardening record](../docs/javascript-hardening.md) describes public lifecycle,
concurrency and hostile-result tests, reproducible measurements, and remaining
release gates. A bounded public-API soak can be run separately from unit tests:

```sh
go run ./cmd/hardening -iterations 1000 -workers 4 -cycles 10
go test -run '^$' -bench '^BenchmarkJavaScript$' -benchmem -benchtime=300ms -count=3
go test -run '^$' -fuzz '^FuzzOwnedJSONConversion$' -fuzztime=500x -parallel=2
```

The soak uses an explicit 8 MiB guest cap to exercise allocation rejection and
checks every result/failure plus a healthy evaluation afterward. Its JSON report
contains timing, checked counts and post-GC Go memory samples. These samples
are not process RSS, a leak proof, or a hard host-memory bound. The default
processor cap remains 64 MiB. Applications should bound concurrent calls outside
the engine; each call creates its own guest and consumes host resources too.

[MIT](LICENSE). Third-party guest components retain their accompanying notices.
