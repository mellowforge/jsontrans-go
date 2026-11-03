//go:build !race

package javascript

import "time"

// Trusted fixture initialization includes race-instrumented Wasm compilation.
// Explicit cancellation-test deadlines and production defaults stay unchanged.
const semanticTestTimeout = 10 * time.Second
