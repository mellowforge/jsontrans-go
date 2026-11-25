package jsonata

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func TestReferenceNumberPictureExponent(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatNumber(2.55,"x0e0x")`, `null`, "ERR D3093"},
		{`$formatNumber(2.55,"\ud8000.0e00\ud800")`, `null`, `"\ud8002.6e0\ud800"`},
		{`$formatNumber(-12.34,"\ud8000.0e00\ud800")`, `null`, `"-\ud8001.2e1\ud800"`},
		{`$formatNumber(1e22,"x0.0e00x")`, `null`, `"x10.0e21x"`},
		{`$formatNumber(2.55,"😀0.0e00😀")`, `null`, "ERR D3093"},
		{`$formatNumber(10,"0e0")`, `null`, `"10e0"`},
		{`$formatNumber(-10,"0.0e0")`, `null`, `"-10.0e0"`},
		{`$formatNumber(0,"0.0e00")`, `null`, `"0.0e00"`},
		{`$formatNumber(0.005,"0.00e00")`, `null`, `"5.00e-03"`},
		{`$formatNumber(10,"0e0;bad")`, `null`, "ERR D3086"},
		{`$formatNumber(20,"#e0")`, `null`, `"0.e2"`},
		{`$formatNumber(0,"#e0")`, `null`, `"0.e0"`},
		{`$formatNumber(20,"##e00")`, `null`, `"0.e02"`},
		{`$formatNumber(1.015,"0.00e0")`, `null`, `"1.02e0"`},
	})
}

func TestReferenceNumberPictureFixedRoundingTies(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatNumber(2355387201299.891,"0.0000000000")`, `null`, `"2355387201299.8911132813"`},
		{`$formatNumber(10000000000000002,"0000000000000000.0e00")`, `null`, `"1000000000000000.3e01"`},
		{`$formatNumber(-10000000000000002,"0000000000000000.0e00")`, `null`, `"-1000000000000000.3e01"`},
		{`$formatNumber(99.99999999999997,"00.000000000000000000000000000000000000000000000e00")`, `null`, `"99.999999999999957367435854393988847732543945313e00"`},
	})
}

func TestReferenceNumberPictureWideExponentBoundaries(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatNumber(1e26,"` + strings.Repeat("0", 26) + `e00")`, `null`, `"` + strings.Repeat("0", 21) + `1e+25e01"`},
		{`$formatNumber(1.0000000000000001e29,"` + strings.Repeat("0", 29) + `.0e00")`, `null`, `"` + strings.Repeat("0", 28) + `1.0000000000000003e+29e00"`},
		{`$formatNumber(1.0000000000000002e150,"` + strings.Repeat("0", 150) + `.0e00")`, `null`, `"` + strings.Repeat("0", 149) + `1.0000000000000002e+149e01"`},
		{`$formatNumber(1e308,"` + strings.Repeat("0", 310) + `e00")`, `null`, `"` + strings.Repeat("0", 302) + `Infinitye-01"`},
	})
}

func TestReferenceNumberPictureFractionalGrouping(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatNumber(1.234567,"0.00,00,00")`, `null`, `"1.23,4567"`},
		{`$formatNumber(1234.123456,"#,##0.0,00,000")`, `null`, `"1,234.1,23456"`},
		{`$formatNumber(1234.56789,"#0,0,0.000,000")`, `null`, `"1,2,3,4.567,,890"`},
		{`$formatNumber(1.2,"0.0#,##")`, `null`, `"1.2,"`},
		{`$formatNumber(1e308,"0.000,000")`, `null`, `"NaN.000,000"`},
		{`$formatNumber(1e20,"#0.000,000,000")`, `null`, `"100000000000000000000.000,000000"`},
		{`$formatNumber(1.2e22,"٠.٠٠,٠٠",{"zero-digit":"٠"})`, `null`, `"١.٢e,+٢٢"`},
		{`$formatNumber(0,"\ud80000\ud8000\ud8000.000\ud800000\ud800",{"grouping-separator":"\ud800"})`, `null`, `"\ud80000\ud8000\ud8000.000\ud800\ud800000"`},
		{`$formatNumber(0,"\ud800#0\ud8000\ud8000.00\ud800000\ud800",{"grouping-separator":"\ud800"})`, `null`, `"00\ud8000\ud800.00\ud800\ud800\ud80000\ud8000"`},
		{`$formatNumber(1.5,"000.0",{"grouping-separator":"00"})`, `null`, `"0000001.5"`},
		{`$formatNumber(1.5,"0\ud800A",{"grouping-separator":"\ud800A"})`, `null`, `"2.\ud800\ud800A"`},
		{`$formatNumber(1234.123456,"#\ud800##0.0\ud80000\ud800000",{"grouping-separator":"\ud800"})`, `null`, `"1\ud800234.1\ud80023456"`},
	})
}

func TestNumberPictureGroupingRespectsOutputLimit(t *testing.T) {
	for _, limit := range []int{3, 4} {
		b, err := budget.New(context.Background(), budget.Limits{MaxOutputBytes: limit})
		if err != nil {
			t.Fatal(err)
		}
		result, err := formatNumber(&CallContext{Budget: b}, 1.2, "0.0#,##", nil)
		b.Close()
		if limit == 3 {
			if !errors.Is(err, budget.ErrLimit) {
				t.Fatalf("got %v, %v, want output limit", result, err)
			}
		} else if err != nil || result != "1.2," {
			t.Fatalf("got %v, %v, want 1.2,", result, err)
		}
	}
}

func TestNumberPictureExponentScalingChargesWork(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxSteps: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, err = formatNumber(&CallContext{Budget: b}, math.SmallestNonzeroFloat64, "0e0", nil)
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v, want step limit", err)
	}
}
