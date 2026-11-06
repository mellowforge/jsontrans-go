# NATS instrumented load results

Median per-run metrics. p99 values below are medians of individual-run p99s, not a pooled p99. End-to-end percentiles include completed inputs only. Always read completion/loss alongside latency. Brief deterministic paced local tests do not establish production capacity.

| Route family | Fan-out | Workers | Strategy | Completion % | Lossless rounds | Processing p50 us | Processing p99 us | E2E p99 us |
|---|---:|---:|---|---:|---:|---:|---:|---:|

Selected rows target5000 messages/sec; summary.csv records achieved offered rate, queue rejection/wait and all1000/sec cases as well. Lossy runs are not SLO improvements. Core NATS, loopback broker, no TLS/persistence/acks.
