# Public prepared JSON APIs: final NATS validation

This directory records the follow-up to the original
[message byte-path experiment](../../assessments/message-byte-path.md). The new
run uses `jsontrans.EvalPreparedJSON` and `Batch.EvalPreparedJSON`, including
preparation and JSON encoding in timed message processing. The direct baseline
uses precompiled `jsontrans.EvalJSON`. All strategies belong to this library.

## Results

All **486 cases were valid**: 1,458,000 inputs, 1,354,398 completed messages and
8,774,196 validated outputs. There were zero transformation, publish, async,
output-validation, pending-drop, unaccounted-input or incomplete-fan-out errors.
The measured broker was removed, independently confirmed through Docker.

**436 cases were lossless.** The other 50 recorded 103,602 application-queue
rejections under overload. Every single-rule case was lossless (162/162), as was
every public prepared/batch case with eight workers (108/108), across both
offered rates and all fan-outs. This does not mean every worker configuration
sustained the offered rate: one-worker 16-rule object routes completed about
51% of inputs on either public prepared path at the 5,000/sec target.

Selected public-API results at a target of 5,000 inputs/sec; medians across three
rounds. Each displayed row completed 100% of inputs in all three rounds:

| Route | Rules | Workers | API | Processing p50 | Processing p99 | End-to-end p99 |
| --- | ---: | ---: | --- | ---: | ---: | ---: |
| Scalar | 1 | 1 | `EvalPreparedJSON` | 26.0 µs | 92.1 µs | 1.18 ms |
| Object | 1 | 1 | `EvalPreparedJSON` | 48.9 µs | 234.3 µs | 4.60 ms |
| Mixed | 1 | 1 | `EvalPreparedJSON` | 45.0 µs | 218.6 µs | 4.65 ms |
| Scalar | 16 | 8 | `Batch.EvalPreparedJSON` | 52.3 µs | 268.3 µs | 0.86 ms |
| Object | 16 | 8 | `Batch.EvalPreparedJSON` | 354.3 µs | 1,277.5 µs | 7.76 ms |
| Mixed | 16 | 8 | `Batch.EvalPreparedJSON` | 320.7 µs | 1,182.7 µs | 2.54 ms |

Processing is the whole message transformation, including all rules and encoding,
not time per rule. Rows use different worker counts and are not a scaling ratio.
Faster processing did not always lower end-to-end tails: for the one-worker,
single-object case, direct `EvalJSON` had a 3.14 ms end-to-end p99 versus 4.60 ms
for individual prepared evaluation, despite slower median processing (102.4 µs
versus 48.9 µs). Queueing, scheduling and instrumentation remain material.

The public APIs passed this integration validation. Use worker/queue limits
appropriate to the application; the measurements do not define an automatic
strategy switch or production throughput guarantee.

- [All measured runs](load/runs.jsonl)
- [Complete summary](load/summary.md), [CSV](load/summary.csv), [JSON](load/summary.json)
- [Runtime and container metadata](load/metadata.json)
- [Exact command and executable hash](load-command.txt)
- [Verification results](verification/checks.tsv)
- [Measurement and verification checksums](SHA256SUMS)

## Source and environment

- Frozen source: `c1c6889876294ee557c5aec5c6eaf86894f47674`.
- Git-exported archive SHA-256:
  `2120d12b5ddfa4daf94d8e59488b3bb2568c01328c6744cb3bcd4c650ff0fd92`.
- Bertram: Ubuntu 24.04 amd64, Intel Core i9-13900K, 128 GiB RAM.
- Verification: Go 1.26.10 and Go 1.27.3. Load executable: Go 1.27.3,
  default decoder, no experimental build tags and no race instrumentation.
- Client affinity: `0,2,4,6,8,10,12,14`; per-case GOMAXPROCS matches worker count.
- Core NATS 2.15.2-alpine, pinned digest from `DefaultImage`, loopback only;
  broker affinity `16,17`, GOMAXPROCS 2, memory limit 512 MiB.

No host service, CPU frequency policy, production default or Go minimum changed.
Code remained frozen during verification and load measurement; later commits
change repository guidance and documentation only. `corpus-inventory.json`
records expressions, actual byte
lengths, independent expected-output sizes and deterministic corpus hashes for
the native routes.

## Acceptance checks

All **30 verification steps passed** before any timed load began:

| Checks | Steps |
| --- | ---: |
| Root, optional JavaScript, messages and NATS: uncached tests, race detection and vet, on both Go versions | 24 |
| Parent and NATS Python report-validation suites | 2 |
| Bounded message strategy fuzzing, 12,000 executions | 1 |
| Real Testcontainers integration test, normal and race builds | 2 |
| Load executable build | 1 |

The integration test exercises all three strategies at fan-out 1/4/16 with four
workers, checking 256 inputs and every output per case. The added regression
failed on the old internal-encoding path and passes with public output-phase
diagnostics, batch failure index, no partial worker outputs, and recovery.
Behavioral tests also cover destination reuse without overwriting retained byte
buffers.

`verification/checks.tsv` records each check's exit status and elapsed seconds;
the adjacent logs retain its results and environment. Commands are listed below. Root
formatting/whitespace checks and local Mac root/message tests, race detection,
vet and message fuzzing also passed. The Mac's default module cache lacked the
pinned NATS dependencies, so Mac NATS setup attempts did not run tests; the
NATS results here are from the complete native Linux checks.

## Load method and interpretation

The unchanged full matrix has 486 measured cases: three rotating rounds, three
strategies, JSONata scalar/object and mixed native routes, fan-out 1/4/16,
workers 1/4/8, and offered rates 1,000/5,000 inputs per second. Each case has a
one-second publication window after a separate 32-message warmup. The 1 KiB
payload size is a minimum target, not an exact serialized size.

Broker startup, image pulls, compilation, connection/subscription setup and
warmups are excluded. Actual processing includes fresh per-message preparation
for prepared strategies and all output encoding. Full values and correlation
IDs are checked; the receiver measures arrival of complete fan-outs. Messages
are paced independently of completion through bounded application queues.

Read completion and queue loss alongside latency. A valid overloaded case can
reject inputs at the application queue; it is not a lossless service result.
Transform, transport and correctness errors remain separate counters. Published
fan-out is not transactional. Processing and end-to-end percentiles are distinct;
end-to-end percentiles include completed messages only. Reported repeated p99s
are medians of per-run p99 values, not pooled quantiles. Client allocation/GC
figures include the instrumented pipeline, not only library evaluation.

These are brief, synthetic, local Core NATS checks without TLS, persistence,
JetStream acknowledgements or remote network traffic. They validate this public
API integration; they do not establish production capacity, universal speedups,
or a memory guarantee. The optional JavaScript adapter is covered by native
module checks, not by this NATS load matrix.

## Reproduce

Repeat the module checks from the repository root with Go 1.26.10 and Go 1.27.3
selected in PATH:

```sh
for module in . javascript experiments/messages experiments/messages/nats; do
  (cd "$module" && go test -count=1 ./... && go test -race -count=1 ./... && go vet ./...)
done
```

With Go 1.27.3 selected, run the parent message fuzz/report checks:

```sh
cd experiments/messages
python3 -m unittest discover -p 'test_*.py'
go test . -run '^$' -fuzz '^FuzzByteStrategies$' -fuzztime=12000x -parallel=2
```

From `experiments/messages/nats`, run report and real-container checks. After all
verification finishes, build and run the load executable:

```sh
python3 -m unittest discover -p 'test_*.py'
go test -tags=integration -run '^TestContainerRoutes$' -count=1 -timeout=5m ./...
go test -race -tags=integration -run '^TestContainerRoutes$' -count=1 -timeout=5m ./...
GOTOOLCHAIN=local GOWORK=off GOEXPERIMENT='' go build -o /tmp/message-load ./cmd/load
taskset -c 0,2,4,6,8,10,12,14 /tmp/message-load \
  -output /tmp/new-public-api-nats-results -rounds 3 -duration 1s
python3 summarize.py /tmp/new-public-api-nats-results
```

The output directory must be new. Broker affinity flags need adjustment on other
hosts. The command must finish successfully, report successful container removal,
and pass the strict summarizer before the data is treated as complete.
