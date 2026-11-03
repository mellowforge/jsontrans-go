# jsontrans-go

**One Go API for JSON queries, transformations, and reusable rules.**

[![Go Reference](https://pkg.go.dev/badge/github.com/mellowforge/jsontrans-go.svg)](https://pkg.go.dev/github.com/mellowforge/jsontrans-go)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![MIT License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

`jsontrans-go` turns a JSON document and an expression into a result. Use
JSONPath to select data, or JSONata to reshape objects, calculate values, and
aggregate collections. Both engines are implemented natively in Go and share
an extensible processor API.

Compile an expression once, prepare a document once, and reuse them wherever
your application needs to evaluate JSON rules.

> **Status:** Native processors are available in
> [v1.1.0](https://github.com/mellowforge/jsontrans-go/releases/tag/v1.1.0).
> JavaScript remains experimental. See the
> [compatibility policy](docs/compatibility.md) for coverage and documented differences.

[Quick start](#quick-start) · [Document reuse](#prepare-once-evaluate-many) ·
[Performance](#performance) · [API reference](https://pkg.go.dev/github.com/mellowforge/jsontrans-go)

## Why jsontrans-go?

- **Two languages, one contract.** Select, filter, calculate, and transform
  through the same compile/evaluate interface.
- **Built for reuse.** Immutable programs and prepared documents support
  concurrent evaluation. Ordered batches evaluate several expressions together.
- **Explicit ownership.** Inputs stay unchanged, and JSON results own their
  mutable data.
- **Controlled execution.** Context cancellation and configurable time, work,
  depth, collection, and size limits are part of the evaluation model.
- **Room to grow.** Register your own processors and language-specific Go
  functions. Each transformer owns its processor registry.

| Processor | Use it for | Compatibility target |
| --- | --- | --- |
| JSONPath | Selecting values from nested documents with paths and filters | RFC 9535 |
| JSONata | Transformations, calculations, aggregation, and expression-based rules | JSONata 2.2.2 |

## Install

Requires **Go 1.26 or newer**. From your application's Go module:

```sh
go get github.com/mellowforge/jsontrans-go
```

The root package is named `jsontrans`. Built-in processors live in the
`jsonpath` and `jsonata` subpackages.

## Quick start

Register the processors your application needs, then evaluate an expression:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func main() {
	ctx := context.Background()

	path, err := jsonpath.New()
	if err != nil {
		log.Fatal(err)
	}
	ata, err := jsonata.New()
	if err != nil {
		log.Fatal(err)
	}
	transformer, err := jsontrans.New(map[string]jsontrans.Processor{
		"jsonpath": path,
		"jsonata":  ata,
	})
	if err != nil {
		log.Fatal(err)
	}

	input := []byte(`{"items":[{"price":4},{"price":7}]}`)
	for _, query := range []struct{ processor, expression string }{
		{"jsonpath", `$.items[*].price`},
		{"jsonata", `$sum(items.price)`},
	} {
		result, err := transformer.ProcessJSON(
			ctx, query.processor, input, query.expression, jsontrans.EvalOptions{},
		)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", query.processor, result.Value)
	}
}
```

```text
jsonpath: [4,7]
jsonata: 11
```

`ProcessJSON` compiles on each call. For a repeated expression, use
`transformer.Compile` and reuse the returned program with `Eval` or `EvalJSON`.
There is no implicit expression cache.

## Prepare once, evaluate many

A prepared document validates and owns the JSON input and builds an immutable
index. Reuse that document across expressions, or compile an ordered batch for
a fixed set of rules.

Using `ctx`, `transformer`, and `input` from the example above:

```go
batch, err := transformer.CompileBatch(ctx, "jsonata", []string{
	`$sum(items.price)`,
	`$count(items)`,
	`items.price`,
})
if err != nil {
	log.Fatal(err)
}

document, err := jsontrans.PrepareJSON(ctx, input, jsontrans.PrepareOptions{})
if err != nil {
	log.Fatal(err)
}
results, err := batch.EvalPrepared(ctx, document, nil, jsontrans.EvalOptions{})
if err != nil {
	log.Fatal(err)
}
fmt.Println(results[0].Value) // 11
```

Pass a reusable `[]jsontrans.Result` destination instead of `nil` to reuse result
slice capacity between sequential batch evaluations. Concurrent calls need
separate destinations. Each expression keeps its own work counters and limits;
a batch also has an overall deadline and aggregate output limit.

Use `jsontrans.EvalPrepared` for an individual compiled program, or
`jsontrans.NewBatch` to combine existing programs from different processors.
Batch errors identify the first failing expression through `BatchError.Index`.
Errors that apply to the whole call, such as invalid options, are returned
directly.

## Performance

The following measurements evaluate **whole batches** of JSONata expressions over a
**1 KiB JSON document**, with a reused result destination and default execution
limits. Compilation is outside the timed evaluation.

| Workload | Expressions | Prepared JSON | Fresh JSON | Prepared memory / allocations |
| --- | ---: | ---: | ---: | ---: |
| Scalar selections and predicates | 64 | 10.04 µs | 15.94 µs | 1,281 B / 68 allocs |
| Projections, filters, and aggregation | 16 | 7.26 µs | 10.82 µs | 3,587 B / 120 allocs |
| Distinct scalar predicates | 64 | 7.66 µs | 13.74 µs | 384 B / 4 allocs |

**Prepared JSON** measures evaluation of an already prepared document.
**Fresh JSON** includes copying, validating, and indexing the input followed by
batch evaluation. Memory figures describe prepared evaluation only.

The simple workload repeats scalar selections and predicates. The mixed
workload adds nested projections, filters, and an arithmetic sum. Distinct
predicates evaluate separate thresholds over a shared path. The fixture has
fixed collection sizes; these measurements describe those specific workloads.

Measured on 2026-11-03 using Go 1.25.13, Apple M5 Max, macOS arm64, and default
GOMAXPROCS=18. Figures are medians of five 300 ms samples per operation on a
shared development machine. Actual latency depends on the document, expression,
and hardware. [Raw measurements and exact commands](docs/benchmarks/2026-11-03.txt).

## Results that preserve meaning

- **JSONPath returns a match list**, including empty and single-match lists.
- **JSONata preserves its result shape.** Missing is `Defined: false`; JSON
  `null` is `Defined: true` with a nil `Value`.
- **JSON byte helpers return `JSONResult`.** Missing has no result bytes;
  a defined null contains the bytes `null`.

The Go-value API accepts `map[string]any`, `[]any`, JSON scalars, finite built-in
numbers, and `json.Number`. JSON decoding preserves numeric precision; JSONPath
preserves selected numbers, and JSONata uses binary64 numbers at its language
boundary. Cycles and unsupported Go types are rejected.

Serialized object member order is retained for JSONata key enumeration. Go maps
use lexical fallback order. Unusual strings and keys containing isolated UTF-16
surrogates use lossless `UTF16String` and `UTF16Object` wrappers.

Errors retain processor, phase, native code, and source-position information.
Wrapped cancellation errors remain discoverable with `errors.Is`.

## Execution controls

Defaults include a **one-second timeout**, **one million work steps**, a
**depth limit of 256**, and **16 MiB input/output limits**. Engines accept
configured defaults; `EvalOptions.Limits` can adjust limits for an individual
call. `EvalOptions.Bindings` supplies JSONata variables.

These are cooperative in-process controls. Registered Go functions must honor
context cancellation and be safe for concurrent calls. See
[execution safety](SECURITY.md) for the full defaults and trust model.

## Compatibility and extensions

Pinned JSONPath and JSONata conformance suites, reference checks, and additional
semantic and concurrency tests track language behavior. The
[compatibility policy](docs/compatibility.md) records coverage, Go-boundary
behavior, and the two explicit JSONPath fixture errata.

Add another processing language by implementing `Processor.Compile` and
`Program.Eval`, then registering it with `jsontrans.New`. Engines can expose
language-specific extension APIs while sharing the common JSON result contract.
The [adapter guide](docs/adapters.md) covers processors, prepared documents,
batches, host functions, and JSONata's raw-value API.

## Explore further

- [Runnable basic example](examples/basic/main.go)
- [Prepared-document example](examples/prepared/main.go)
- [API reference](https://pkg.go.dev/github.com/mellowforge/jsontrans-go)
- [Contributing and development commands](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [JavaScript feasibility assessment](docs/javascript-assessment.md) (experimental)
- [Owned QuickJS runtime checkpoint](docs/quickjs-runtime-foundation.md)

## License

[MIT](LICENSE). Vendored conformance fixtures retain their upstream licenses
and provenance notices.
