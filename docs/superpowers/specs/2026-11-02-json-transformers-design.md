# JSON transformer design

Build a pure Go library with owned native JSONPath RFC 9535 and JSONata 2.2.2
engines. The common API compiles expressions and evaluates JSON-shaped inputs.
External Go packages can register future processors, including Tengo, without
changes to the core. No complete-engine dependencies, JavaScript runtime,
worker service, pipeline system, streaming API, or automatic expression cache.

The common Result distinguishes absent values from null. JSONPath always
returns a list. JSONata retains its sequence flattening and explicit-array
semantics and uses binary64 numbers. Input decoding and JSONPath preserve
numbers. Go values and JSON byte helpers are supported; arbitrary structs are
not automatically marshaled. Language-specific typed extension APIs support
registered Go functions; JSONata additionally supports per-call bindings.

Programs are immutable and concurrent-safe, and inputs remain caller-owned.
Parsing, conversion, evaluation, and built-ins have cooperative budgets:
1 second timeout, 64 KiB expression, depth 256, 1 million steps, 100,000 items
per collection, 16 MiB input/output, 50 ms regex timeout. Limits are configurable.
Host functions must honor context; these controls do not provide OS isolation.

Use independent parsers and evaluators, with a JSONata Pratt parser and AST
interpreter rather than a universal AST or VM. Own semantic compatibility;
focused pure Go supporting dependencies are allowed. Node is a test oracle only.
Pin fixture revisions with provenance. No releases until both complete suites
and added compatibility, resource, concurrency, and fuzz checks pass.

Module: github.com/mellowforge/jsontrans-go. Minimum Go 1.25. License: MIT.
