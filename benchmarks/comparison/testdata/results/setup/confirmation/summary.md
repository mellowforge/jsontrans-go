# Alternating-order setup confirmation

Five 300ms samples per row; before/current order reverses each round.
Same compiled harness binaries; 1 KiB fixtures, reused destination, production defaults.

| Workload | Rules | Before prepare+batch µs | After prepare+batch µs | Warmed gnata µs | Before reuse µs | After reuse µs |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| simple | 64 | 16.49 | 17.66 | 13.96 | 12.33 | 11.37 |
| mixed | 16 | 11.62 | 11.30 | 9.38 | 7.45 | 7.21 |
| unique | 64 | 13.39 | 11.99 | 11.43 | 8.99 | 7.75 |
