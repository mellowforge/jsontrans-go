//go:build !race

package conformance

// raceTimeScale is 1 without the race detector; see race_on_test.go.
const raceTimeScale = 1
