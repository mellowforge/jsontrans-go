# Registry limit forwarding — 2026-11-03

The registry previously expanded partial evaluation overrides into every default
limit field. An experimental JavaScript program then rejected synthesized
`MaxSteps` and `RegexTimeout` values as explicit unsupported requests, even for a
timeout-only override. Custom program defaults could trigger the same failure.

The additive `EvalLimitSupport` interface lets a program declare unsupported
fields. Wrappers snapshot the declaration, reject explicit unsupported requests,
and omit those fields from forwarded copies. `Processor`, `Program` and
`EvalOptions` remain unchanged. Existing programs retain their previous behavior.
Common conversion budgets and native execution budgets remain in force.
Prepared external dispatch also retains configured program/wrapper intersections;
callback mutation of forwarded limits cannot raise the common output budget.

Regression coverage includes registry evaluation, JSON helpers, prepared
fallback and capability dispatch, batches, nested wrappers, concurrent reuse,
all eight fields, caller ownership, explicit default-valued requests and
cancellation. Batches reject unsupported requests before any callback runs.

The owned QuickJS assessment now passes **18/18** probes in strict mode. Raw
results are in [registry/probes.json](../experiments/javascript/testdata/results/registry/probes.json).
The original [checkpoint](quickjs-runtime-foundation.md) and its recorded results
remain available. Run from `experiments/javascript`:

```sh
CGO_ENABLED=0 go build -o /tmp/jsontrans-registry-assess ./cmd/assess
python3 run_probes.py --binary /tmp/jsontrans-registry-assess --backend owned-quickjs --strict --output /tmp/registry-probes.json
```

A native batch smoke check on Go 1.26.10, macOS arm64 / Apple M5 Max retained
**3,584 B/op and 120 allocs/op** before and after the change. Three 100 ms samples
were 8.16–8.84 microseconds before and 8.26–8.41 microseconds after; these short
runs establish allocation continuity, not a statistically significant speedup.
Baseline: merged commit `4a56fc1755e0b770f377be49ee57c2205e3efda6`.
Run from `benchmarks/comparison`:

```sh
GOMAXPROCS=1 go test -tags 'batch prepared' -run '^$' -bench '^BenchmarkCompiledBatch$/^8192$/^mixed$/^N16$/^reused_doc$/^reused_dst$' -benchmem -benchtime=100ms -count=3 -cpu=1
```

This completes the registry integration gate, not production JavaScript support.
JS instruction counting, separate regex deadlines and explicit stack quotas are
still unsupported. The owned runtime's time-snapshot profile, guest memory cap
and process-memory distinction are unchanged. Production diagnostics, source
mapping, lifecycle design and the optional public adapter remain separate work.
