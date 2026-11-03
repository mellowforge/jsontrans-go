//go:build !darwin && !linux

package javascript

func peakRSS() uint64 { return 0 }
