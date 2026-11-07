# Public API NATS validation implementation plan

> **For agentic workers:** Use superpowers:executing-plans for inline implementation and one final fresh review.

**Goal:** Exercise the new public prepared JSON-result APIs through the existing NATS harness, validate on Bertram, then publish the branch and open a PR with gh.

**Architecture:** The development-only message Worker uses `EvalPreparedJSON` or `Batch.EvalPreparedJSON` instead of Go-result evaluation plus an internal encoder. Keep the shared message deadline, aggregate byte limit, worker-local destination, and direct `EvalJSON` baseline. The existing nested NATS module consumes that Worker without changing transport behavior.

**Tech Stack:** Go 1.26+, pinned Core NATS/Testcontainers dependencies, local SSH/Docker verification on Bertram.

**Spec:** Approved user instruction on 2026-11-07; API contract in `docs/prepared-json-results.md` and experiment requirements in `docs/superpowers/plans/2026-11-06-message-byte-path.md`.

## Global constraints

- No production API, defaults, guest, dependency pin, or minimum Go version changes.
- Use the public byte-result APIs, preserve diagnostics and error identity, no partial results after failure.
- Preserve input and byte ownership, missing/null, configured limits and bindings.
- Preserve pinned loopback-only broker, resource limits, bounded queues, warmup exclusion, full output validation, and cleanup.
- Keep historical results labeled with their original source; record new source identity and commands separately.

## Review focus

Aggregate encoded-byte failures retain public output diagnostics and batch indexes. Failure recovery reuses a clean worker. Caller-held byte buffers survive destination reuse. Whole-message deadlines include preparation and evaluation. Load reports must account for all inputs and never hide drops or incomplete output.

## Task 1: Public API dispatch

Files: `experiments/messages/plan.go`, `plan_test.go`, parent/NATS READMEs, load metadata.

- [ ] Add a regression asserting public batch output-limit diagnostics and worker recovery; observe failure on the old internal encoding path.
- [ ] Replace prepared evaluation/encoding with public helpers, remove intermediate Go-tree scratch, preserve outer controls and clear-on-error behavior.
- [ ] Add behavioral ownership/destination reuse coverage; identify public API entry points in run metadata.
- [ ] Run parent and NATS unit tests/race/vet, root/JavaScript checks, formatting and bounded message fuzzing.

## Task 2: Linux acceptance and publication

- [ ] Freeze/commit source and transfer a metadata-free archive to an isolated `/data/ci` checkout.
- [ ] Run Linux root, JavaScript, message and NATS tests/race/vet; run the tagged real-container integration test normally and with race detection.
- [ ] After verification finishes, run the unchanged full NATS matrix on Go 1.27.3: three rotating rounds, all strategies, fan-out 1/4/16, workers 1/4/8, rates 1000/5000, one-second windows.
- [ ] Validate/summarize every case, confirm broker removal, retain raw evidence and report final single-rule/fan-out results and limitations.
- [ ] Obtain one fresh review, resolve blocking findings, commit evidence, push and open a PR using gh. Do not merge or release.
