//go:build race

package quickjswasm

import "time"

// Trusted fixture initialization includes race-instrumented Wasm compilation.
// Explicit cancellation-test deadlines and production defaults stay unchanged.
const semanticTestTimeout = 100 * time.Second
