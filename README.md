# jsontrans-go

A pure Go JSON processing library with an extensible processor API and native
JSONPath and JSONata engines.

Compatibility targets are JSONPath RFC 9535 and JSONata 2.2.2. The pinned
compatibility suites and reference comparisons pass; see the compatibility
record for coverage and the two explicit JSONPath fixture errata.
No release has been published.

```go
path, err := jsonpath.New()
if err != nil { return err }
ata, err := jsonata.New()
if err != nil { return err }
transformer, err := jsontrans.New(map[string]jsontrans.Processor{
    "jsonpath": path,
    "jsonata": ata,
})
if err != nil { return err }

result, err := transformer.ProcessJSON(ctx, "jsonata",
    []byte(`{"items":[{"price":4},{"price":7}]}`),
    `$sum(items.price)`, jsontrans.EvalOptions{})
// A successful result has Defined=true and Value containing JSON 11.
```

Imports: `github.com/mellowforge/jsontrans-go`, and its `/jsonpath` and `/jsonata`
subpackages. Go 1.25 or newer is required. Run `go run ./examples/basic` for a
complete example.

Use `Compile(ctx, processorName, expression)` and `Program.Eval(ctx, input,
options)` for repeated expressions. Programs support concurrent evaluations;
registered Go functions must also be safe for concurrent calls. Registration is
explicit and local to a transformer. There is no implicit expression cache.

For many expressions over the same JSON, call `PrepareJSON(ctx, bytes,
PrepareOptions{})` once, then `EvalPrepared(ctx, program, document,
EvalOptions{})` for each compiled program. Documents own their input and can be
shared concurrently. Native engines use indexed reads for supported expressions
and materialize detached input for the interpreter otherwise. Results remain
detached. See [the prepared example](examples/prepared/main.go) and
[measured costs and remaining performance gaps](docs/performance.md).

For a fixed set of rules, `transformer.CompileBatch(ctx, "jsonata", expressions)`
compiles an ordered batch. Call `batch.EvalPrepared(ctx, document, dst, options)`
to reuse a `[]jsontrans.Result` destination across documents. `NewBatch(programs)`
also accepts already compiled programs from different processors. Each call has
an overall deadline and keeps each expression's configured limits. Batch errors
identify the first failing expression with a zero-based `BatchError.Index`;
failures not tied to an expression, such as invalid options, are returned
unwrapped.

The Go-value API accepts `map[string]any`, `[]any`, nil, strings, booleans,
finite built-in numeric types, and `json.Number`. It rejects cycles, arbitrary
structs, and other unsupported types. Inputs and returned JSON values are
detached. JSON decoding preserves numbers; JSONata converts them to binary64.

JSONPath returns a list for zero, one, or many matches. JSONata preserves its
language's result shapes. `Result.Defined` distinguishes missing from JSON null;
JSON null has `Defined=true` and `Value=nil`. JSON byte helpers return
`JSONResult`, with absent bytes for missing and `null` bytes for null.

JSON byte helpers retain serialized object member order for JSONata key
enumeration. Plain Go maps have no source order, so their fallback is lexical.
Isolated UTF-16 surrogate units use `UTF16String` result values and, for unusual
object keys, `UTF16Object`; both implement lossless JSON marshaling. Ordinary
Unicode strings and objects remain `string` and `map[string]any`.

See [the adapter guide](docs/adapters.md), [compatibility policy](docs/compatibility.md),
[contributing](CONTRIBUTING.md), and [execution safety](SECURITY.md).
Initial measurements are in [the performance baseline](docs/performance.md).

License: MIT. Upstream conformance data carries its original notices.
