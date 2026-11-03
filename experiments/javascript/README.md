# JavaScript feasibility assessment

This optional development module evaluates synchronous function-expression
transformers using QuickJS/Wazero and Goja. It is **not a supported processor or
sandbox**. Both candidates currently fail required gates; see the
[assessment and production plan](../../docs/javascript-assessment.md).

The root library imports neither candidate. JSONPath and JSONata remain native
Go. The assessment reuses `Processor`, `Program`, registry, JSON helpers and the
prepared-document fallback without changing their public interfaces.

Programs compile trusted conversion code independently from user source, then
create a fresh runtime on every evaluation. Both compiled artifacts are immutable. Inputs and
bindings cross a detached JSON text boundary. Numbers use JavaScript binary64;
top-level `undefined` is missing and `null` is defined. The bridge rejects cycles,
functions, promises, BigInt, symbols, nonfinite numbers, accessors, non-plain
objects, sparse arrays and extra array properties. No event loop or job queue is
run. No host functions, Node APIs, runtime imports or runtime pooling are added.
Prebundled helper functions work inside the supplied function expression.

The experimental API deliberately rejects explicitly supplied `MaxSteps` and
`RegexTimeout` evaluation overrides: it cannot promise JS instruction counting
or a separate per-regex deadline. Default step accounting applies to Go/bridge
conversion, **not JS execution**. Wall-clock cancellation and QuickJS allocator
limits are evaluated separately. Other unsupported controls, structured error
positions and resolved-option forwarding still require production work. Do not
use this module for untrusted execution.

## Reproduce

Requires Go 1.26 (preferred toolchain 1.26.10), Python 3 for probe orchestration,
and the initial Go module downloads. Normal Go builds require no Node, C compiler,
WASI SDK or network once dependencies/toolchain are cached. Run from this folder:

```sh
go mod download
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -o /tmp/jsontrans-js-assess ./cmd/assess
python3 run_probes.py
# Expected nonzero: current candidates fail acceptance gates.
python3 run_probes.py --strict

go test -run '^$' -fuzz '^FuzzStringConversion$' -fuzztime=1000x -parallel=2
GOMAXPROCS=1 go test -run '^$' -bench . -benchmem -benchtime=200ms -count=5 -cpu=1 > testdata/results/bench.txt
python3 summarize.py
```

Probe subprocesses have a four-second external watchdog. An expired evaluation
has a 75 ms deadline; startup and compilation are separate. Goja's uncapped
allocation-growth probe is recorded as a **failed gate by audit**, without
executing it. Every other case is executed or explicitly marked inapplicable;
failures are retained in JSON, including watchdog kills. `--strict` makes gate
failure visible to automation. Ordinary unit tests avoid deliberate hangs.
Trusted semantic fixtures raise the timeout to 10 seconds so race-instrumented
Wasm compilation does not consume the production one-second default.

Import audit and artifact fingerprint:

```sh
qjs_dir=$(go list -m -f '{{.Dir}}' github.com/fastschema/qjs)
/tmp/jsontrans-js-assess -audit-wasm "$qjs_dir/qjs.wasm" > testdata/results/wasm-audit.json
```

Fresh-process initialization and peak RSS (five samples; no disk compilation cache):

```sh
python3 measure_cold.py
```

Stripped binary sizes (same toolchain/platform, no CGO):

```sh
for command in bare goja-min quickjs-min assess; do
  CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "/tmp/js-$command" "./cmd/$command"
  wc -c "/tmp/js-$command"
done
```

`cmd/goja-min` and `cmd/quickjs-min` isolate the embedded engine's minimum size
cost. The full assessment binary links **both** engines and the root helpers;
it is not the binary cost of a future single-engine production adapter.

Recorded measurements are in [testdata/results](testdata/results). RSS is
process-wide, includes Go and executable/compiler memory, and is distinct from
Wasm linear memory. B/op measures cumulative Go allocations per operation,
not retained memory or a guest-memory ceiling. Cross-build checks used
`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`, `linux/arm64` and `windows/amd64`;
these builds were not executed on those targets.

## Pinned dependencies and licensing

See [go.mod](go.mod), [go.sum](go.sum) and the build dependency manifest in
[testdata/results/modules.txt](testdata/results/modules.txt). QJS v0.0.6 includes
a 1,038,767-byte Wasm artifact; its SHA-256 and imports are recorded by the audit.
QJS, QuickJS-ng, Goja and regexp2 use MIT licenses; Wazero uses Apache-2.0;
Go text, sourcemap and pprof carry BSD licenses. Original licenses remain in their
module source distributions. No upstream engine source or artifact is copied into
this repository. Rebuilding the Wasm engine requires an upstream source checkout
with its submodule, WASI SDK, CMake, Make and Binaryen `wasm-opt`; production
must establish a reproducible artifact build and provenance before adoption.
