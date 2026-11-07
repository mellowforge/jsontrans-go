# NATS instrumented load results

Median per-run metrics. p99 values below are medians of individual-run p99s, not a pooled p99. End-to-end percentiles include completed inputs only. Always read completion/loss alongside latency. Brief deterministic paced local tests do not establish production capacity.

| Route family | Fan-out | Workers | Strategy | Completion % | Lossless rounds | Processing p50 us | Processing p99 us | E2E p99 us |
|---|---:|---:|---|---:|---:|---:|---:|---:|
| scalar | 1 | 1 | direct | 100.0 | 3/3 | 61.8 | 191.5 | 3246.0 |
| scalar | 1 | 1 | prepared | 100.0 | 3/3 | 26.0 | 92.1 | 1181.8 |
| scalar | 1 | 1 | batch | 100.0 | 3/3 | 25.4 | 87.6 | 1100.3 |
| scalar | 1 | 8 | direct | 100.0 | 3/3 | 184.3 | 657.0 | 1199.6 |
| scalar | 1 | 8 | prepared | 100.0 | 3/3 | 31.7 | 235.0 | 736.4 |
| scalar | 1 | 8 | batch | 100.0 | 3/3 | 31.3 | 232.7 | 734.4 |
| scalar | 16 | 1 | direct | 20.6 | 0/3 | 851.6 | 2266.4 | 141853.1 |
| scalar | 16 | 1 | prepared | 100.0 | 3/3 | 41.0 | 146.7 | 4997.8 |
| scalar | 16 | 1 | batch | 100.0 | 3/3 | 35.8 | 117.8 | 3621.5 |
| scalar | 16 | 8 | direct | 100.0 | 3/3 | 1615.4 | 3349.6 | 76117.1 |
| scalar | 16 | 8 | prepared | 100.0 | 3/3 | 68.0 | 312.7 | 929.4 |
| scalar | 16 | 8 | batch | 100.0 | 3/3 | 52.3 | 268.3 | 856.7 |
| object | 1 | 1 | direct | 100.0 | 3/3 | 102.4 | 258.0 | 3141.1 |
| object | 1 | 1 | prepared | 100.0 | 3/3 | 48.9 | 234.3 | 4598.9 |
| object | 1 | 1 | batch | 100.0 | 3/3 | 48.4 | 230.9 | 4699.2 |
| object | 1 | 8 | direct | 100.0 | 3/3 | 289.9 | 939.8 | 1712.5 |
| object | 1 | 8 | prepared | 100.0 | 3/3 | 111.1 | 584.0 | 1158.4 |
| object | 1 | 8 | batch | 100.0 | 3/3 | 113.3 | 614.0 | 1202.9 |
| object | 16 | 1 | direct | 10.7 | 0/3 | 1888.8 | 3572.9 | 202419.2 |
| object | 16 | 1 | prepared | 51.7 | 0/3 | 277.0 | 1092.7 | 60683.2 |
| object | 16 | 1 | batch | 51.3 | 0/3 | 274.2 | 1027.8 | 73088.2 |
| object | 16 | 8 | direct | 57.8 | 0/3 | 3274.1 | 5063.1 | 222640.9 |
| object | 16 | 8 | prepared | 100.0 | 3/3 | 356.0 | 1268.8 | 8105.5 |
| object | 16 | 8 | batch | 100.0 | 3/3 | 354.3 | 1277.5 | 7758.6 |
| mixed | 1 | 1 | direct | 100.0 | 3/3 | 61.5 | 236.7 | 3018.1 |
| mixed | 1 | 1 | prepared | 100.0 | 3/3 | 45.0 | 218.6 | 4649.5 |
| mixed | 1 | 1 | batch | 100.0 | 3/3 | 43.8 | 211.6 | 3856.8 |
| mixed | 1 | 8 | direct | 100.0 | 3/3 | 255.0 | 935.9 | 1443.1 |
| mixed | 1 | 8 | prepared | 100.0 | 3/3 | 79.0 | 500.7 | 999.5 |
| mixed | 1 | 8 | batch | 100.0 | 3/3 | 78.1 | 504.5 | 1020.0 |
| mixed | 16 | 1 | direct | 16.8 | 0/3 | 1135.3 | 3139.3 | 162190.1 |
| mixed | 16 | 1 | prepared | 98.5 | 0/3 | 147.5 | 658.7 | 40432.5 |
| mixed | 16 | 1 | batch | 100.0 | 2/3 | 139.7 | 644.0 | 16729.0 |
| mixed | 16 | 8 | direct | 82.3 | 0/3 | 1889.4 | 4623.3 | 145793.1 |
| mixed | 16 | 8 | prepared | 100.0 | 3/3 | 329.2 | 1282.2 | 4723.1 |
| mixed | 16 | 8 | batch | 100.0 | 3/3 | 320.7 | 1182.7 | 2535.3 |

Selected rows target5000 messages/sec; summary.csv records achieved offered rate, queue rejection/wait and all1000/sec cases as well. Lossy runs are not SLO improvements. Core NATS, loopback broker, no TLS/persistence/acks.
