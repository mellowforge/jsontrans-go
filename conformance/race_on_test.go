//go:build race

package conformance

// raceTimeScale stretches fixture wall-clock limits under the race detector,
// which slows evaluation several times on shared CI runners. No fixture
// expects a time-limit error; expected U1001 errors come from depth guards.
const raceTimeScale = 10
