# Deadline-pass matched measurements

Five 200 ms samples per row; medians in microseconds per document/batch.
Preparation-inclusive rows reuse the destination; all safety defaults active.

| KiB | Workload | Rules | Before prepare+batch | After prepare+batch | Warmed gnata | Before reused doc | After reused doc | After B/allocs (reused doc) |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | simple | 4 | 5.79 | 4.64 | 2.99 | 1.44 | 0.98 | 552/11 |
| 1 | simple | 16 | 9.09 | 6.76 | 4.77 | 4.73 | 3.07 | 752/20 |
| 1 | simple | 64 | 22.97 | 15.87 | 11.23 | 17.96 | 11.32 | 1,281/68 |
| 1 | mixed | 16 | 14.02 | 11.24 | 8.89 | 9.52 | 7.30 | 3,588/120 |
| 1 | unique | 64 | 20.48 | 13.29 | 11.32 | 15.64 | 8.89 | 384/4 |
| 8 | mixed | 16 | 18.71 | 17.26 | 17.82 | 9.51 | 7.47 | 3,588/120 |
| 64 | simple | 64 | 63.73 | 57.17 | 157.77 | 18.26 | 11.46 | 1,281/68 |
