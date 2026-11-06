# NATS instrumented load results

Median per-run metrics. p99 values below are medians of individual-run p99s, not a pooled p99. End-to-end percentiles include completed inputs only. Always read completion/loss alongside latency. Brief deterministic paced local tests do not establish production capacity.

| Route family | Fan-out | Workers | Strategy | Completion % | Lossless rounds | Processing p50 us | Processing p99 us | E2E p99 us |
|---|---:|---:|---|---:|---:|---:|---:|---:|
| scalar | 1 | 1 | direct | 100.0 | 3/3 | 60.7 | 199.4 | 2861.4 |
| scalar | 1 | 1 | prepared | 100.0 | 3/3 | 24.4 | 84.9 | 1063.1 |
| scalar | 1 | 1 | batch | 100.0 | 3/3 | 25.1 | 93.0 | 1147.2 |
| scalar | 1 | 8 | direct | 100.0 | 3/3 | 182.5 | 651.2 | 1191.3 |
| scalar | 1 | 8 | prepared | 100.0 | 3/3 | 30.7 | 200.0 | 666.4 |
| scalar | 1 | 8 | batch | 100.0 | 3/3 | 30.5 | 171.0 | 663.4 |
| scalar | 16 | 1 | direct | 20.3 | 0/3 | 850.5 | 2239.3 | 141817.7 |
| scalar | 16 | 1 | prepared | 100.0 | 3/3 | 38.4 | 132.1 | 4598.8 |
| scalar | 16 | 1 | batch | 100.0 | 3/3 | 34.6 | 126.6 | 5115.1 |
| scalar | 16 | 8 | direct | 100.0 | 3/3 | 1600.8 | 3023.4 | 58255.1 |
| scalar | 16 | 8 | prepared | 100.0 | 3/3 | 57.7 | 261.6 | 832.8 |
| scalar | 16 | 8 | batch | 100.0 | 3/3 | 48.2 | 231.4 | 824.6 |
| object | 1 | 1 | direct | 100.0 | 3/3 | 101.6 | 261.8 | 3162.4 |
| object | 1 | 1 | prepared | 100.0 | 3/3 | 46.9 | 210.5 | 4194.9 |
| object | 1 | 1 | batch | 100.0 | 3/3 | 48.4 | 226.2 | 4047.6 |
| object | 1 | 8 | direct | 100.0 | 3/3 | 293.8 | 957.7 | 1589.1 |
| object | 1 | 8 | prepared | 100.0 | 3/3 | 109.6 | 585.3 | 1177.8 |
| object | 1 | 8 | batch | 100.0 | 3/3 | 111.7 | 589.7 | 1161.3 |
| object | 16 | 1 | direct | 10.8 | 0/3 | 1827.8 | 3513.9 | 202701.5 |
| object | 16 | 1 | prepared | 55.1 | 0/3 | 251.1 | 1245.4 | 59518.2 |
| object | 16 | 1 | batch | 53.9 | 0/3 | 259.4 | 1137.9 | 60786.2 |
| object | 16 | 8 | direct | 57.7 | 0/3 | 3267.8 | 5438.0 | 222071.4 |
| object | 16 | 8 | prepared | 100.0 | 3/3 | 339.8 | 1180.6 | 5536.4 |
| object | 16 | 8 | batch | 100.0 | 3/3 | 349.2 | 1217.0 | 6572.9 |
| mixed | 1 | 1 | direct | 100.0 | 3/3 | 65.4 | 255.7 | 3573.9 |
| mixed | 1 | 1 | prepared | 100.0 | 3/3 | 42.6 | 194.5 | 3805.5 |
| mixed | 1 | 1 | batch | 100.0 | 3/3 | 45.3 | 223.5 | 3977.6 |
| mixed | 1 | 8 | direct | 100.0 | 3/3 | 250.9 | 940.3 | 1490.4 |
| mixed | 1 | 8 | prepared | 100.0 | 3/3 | 75.2 | 476.7 | 1035.5 |
| mixed | 1 | 8 | batch | 100.0 | 3/3 | 76.9 | 498.0 | 1027.5 |
| mixed | 16 | 1 | direct | 16.7 | 0/3 | 1143.5 | 3148.7 | 162100.4 |
| mixed | 16 | 1 | prepared | 99.2 | 1/3 | 141.8 | 661.7 | 37750.0 |
| mixed | 16 | 1 | batch | 99.6 | 1/3 | 141.5 | 659.0 | 39162.8 |
| mixed | 16 | 8 | direct | 82.2 | 0/3 | 1881.2 | 5071.9 | 149669.7 |
| mixed | 16 | 8 | prepared | 100.0 | 3/3 | 320.3 | 1263.1 | 4258.0 |
| mixed | 16 | 8 | batch | 100.0 | 3/3 | 322.4 | 1266.9 | 3347.1 |

Selected rows target5000 messages/sec; summary.csv records achieved offered rate, queue rejection/wait and all1000/sec cases as well. Lossy runs are not SLO improvements. Core NATS, loopback broker, no TLS/persistence/acks.
