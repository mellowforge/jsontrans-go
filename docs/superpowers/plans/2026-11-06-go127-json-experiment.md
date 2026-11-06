# Go 1.27 JSON experiment

Approved scope: compare unchanged library on Go 1.26.10, Go 1.27.3, and Go 1.27.3 with `GOEXPERIMENT=nojsonv2`; prototype jsontext decoding at the existing bounded conversion boundary. Keep the public contracts, Go minimum, native engines, prepared scanner/index, and JavaScript runtime unchanged.

1. Establish a clean baseline and isolated branch. Add correctness-checked native benchmarks for decode/encode, preparation, ordinary JSON evaluation, and prepared 1/16/64-expression workloads, with 1/8/64 KiB and larger array fixtures. Include JavaScript conversion/fallback measurements separately.
2. Add differential semantic, ownership, cancellation, depth/item/byte/work-limit tests before implementing the decoder. Preserve duplicate-name last-write semantics, number spelling, UTF-16 code units, and object-order provenance. Keep final Clone validation/accounting unchanged to isolate parser effects.
3. Implement a `jsontext_experiment` build-tagged decoder on Go 1.27 with jsonv2 enabled. Default and nojsonv2 builds use the existing decoder. Do not silently claim the prototype ran when the required API is unavailable.
4. Add reproducible benchmark runner and raw results with toolchain/environment metadata. Compile outside measured processes; report ns/op, B/op, allocs/op and whole-benchmark-process high-water RSS with its limitations. Repeat sequential runs; do not benchmark competing configurations concurrently.
5. Run formatting, root tests/race/vet for baseline and prototype, pinned conformance, bounded decoder/document fuzzing, and relevant optional JavaScript checks. Document measured results, remaining limitations, and recommendation. Do not adopt a backend or publish a release in this experiment.

The specimen is a materializing decoder, not a streaming evaluator or a zero-allocation guarantee. Check cancellation between tokens and retain existing bounded-but-uninterruptible individual token scanning behavior. No claim of hard process-memory control.
