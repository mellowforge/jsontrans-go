# Adding a processor

An external Go package implements `jsontrans.Processor` and returns a
`jsontrans.Program` from `Compile`. Programs accept JSON-shaped values and
evaluation options and return `jsontrans.Result`. Constructors and syntax stay
in the adapter package; the root package never imports adapters.

Register instances explicitly with `jsontrans.New(map[string]jsontrans.Processor{
"custom": processor})`. The transformer snapshots the map. Use an optional
`Limits() jsontrans.Limits` method on engines and programs to expose configured
defaults to common wrappers. Zero limit fields select library defaults.

Compiled state must be immutable. Create evaluation state per call; snapshot
configuration and function registrations when compiling. Do not mutate inputs
or bindings. Honor context, validate JSON results, and reject unsupported options
instead of silently ignoring them. Preserve `errors.Is` and native diagnostics
through `jsontrans.Error` where applicable.

The common API only accepts JSON-compatible results. Language-specific raw
evaluation may expose function values separately. Missing is not an error and
must use `Defined=false`; null is a defined nil. Each adapter documents its own
result shape, numeric semantics, bindings, and host-function API.

A future Tengo adapter can retain compiled bytecode and create a fresh VM for
each evaluation behind this contract. Tengo is not a v1 dependency or engine.

JSONata exposes `CompileRaw`, `EvalRaw`, `Callable`, and `Invoke` for language
values such as functions. Invoke creates a fresh bounded evaluation so returned
closures and transforms remain usable after their original evaluation finishes.
Register host functions with `jsonata.WithFunction`; JSONPath's `WithFunction`
uses its RFC Value/Logical/Nodes type declarations. These are language-specific
extension APIs, separate from the common JSON contract.
