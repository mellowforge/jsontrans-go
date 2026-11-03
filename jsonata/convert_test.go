package jsonata

import (
	"math"
	"testing"
)

// Go leaves out-of-range float conversions implementation-defined (arm64
// saturates, amd64 does not); floatToInt must give the same answer everywhere.
func TestFloatToIntIsPlatformIndependent(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want int
	}{
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt},
		{math.Inf(-1), math.MinInt},
		{1e300, math.MaxInt},
		{-1e300, math.MinInt},
		{2.9, 2},
		{-2.9, -2},
	} {
		if got := floatToInt(tt.in); got != tt.want {
			t.Errorf("floatToInt(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
	if got := floatToInt64(math.Inf(1)); got != math.MaxInt64 {
		t.Errorf("floatToInt64(+Inf) = %d", got)
	}
}
