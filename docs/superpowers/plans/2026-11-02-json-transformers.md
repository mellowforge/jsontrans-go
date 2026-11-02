# JSON transformer implementation plan

Spec: ../specs/2026-11-02-json-transformers-design.md

1. Establish the module, project rules, contributor/security documentation,
   CI, adapter contract, and pinned offline conformance runners.
2. Implement bounded input/output conversion, numeric preservation, errors,
   immutable registration, JSON helpers, cancellation, and execution budgets.
3. Implement full RFC 9535 JSONPath, including filters, function types and
   extensions, I-Regexp, duplicates, and permitted result ordering.
4. Implement JSONata lexer, Pratt parser, language values, sequence semantics,
   paths, scopes, tuples, grouping, sorting, operators, and transformations.
5. Complete closures, higher-order functions, signatures, partial application,
   tail calls, nested eval, regex, and all standard-library functions.
6. Resolve conformance failures; run differential, adversarial, race, fuzz,
   formatting, vet, and benchmark checks. Document measured compatibility and
   examples. Do not publish or tag before all release gates pass.

The public contract is Processor.Compile(ctx, expression) (Program, error),
Program.Eval(ctx, input, EvalOptions) (Result, error), and Result{Value, Defined}.
Transformer owns a snapshot registry and exposes Process, Compile, and
ProcessJSON; EvalJSON supports compiled programs. Constructors reside in
engine packages. Default limits and architecture are fixed by the spec.
