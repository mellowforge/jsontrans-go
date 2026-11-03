# Batch setup matched measurements

Five 200 ms samples per row; medians in microseconds per document/batch.
Preparation-inclusive rows reuse the destination; all safety defaults active.

| KiB | Workload | Rules | Before prepare+batch | After prepare+batch | Warmed gnata | Before reused doc | After reused doc | After B/allocs (reused doc) |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | simple | 4 | 4.66 | 4.54 | 2.97 | 0.98 | 0.90 | 552/11 |
| 1 | simple | 16 | 8.29 | 7.14 | 4.84 | 3.08 | 3.10 | 752/20 |
| 1 | simple | 64 | 16.09 | 15.49 | 12.34 | 11.47 | 10.22 | 1,281/68 |
| 1 | mixed | 16 | 11.42 | 14.53 | 11.91 | 7.33 | 9.20 | 3,587/120 |
| 1 | unique | 64 | 14.43 | 14.91 | 15.57 | 11.33 | 7.67 | 384/4 |
| 8 | mixed | 16 | 20.95 | 21.39 | 24.12 | 10.05 | 8.76 | 3,588/120 |
| 64 | simple | 64 | 57.44 | 55.99 | 163.11 | 11.95 | 10.24 | 1,281/68 |
