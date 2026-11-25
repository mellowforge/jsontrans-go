package jsonata

import (
	"context"
	"errors"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func TestReferenceUTF16NumberPictures(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatNumber(1234.56,"\ud800#,##0.00\udfff")`, `null`, `"\ud8001,234.56\udfff"`},
		{`$formatNumber(1234.56,"\ud804😀#,##0.00\ue000\ud801")`, `null`, "\"\\ud804😀1,234.56\ue000\\ud801\""},
		{`$formatNumber(-5.5,"0.00",{"minus-sign":"\ud800"})`, `null`, `"\ud8005.50"`},
		{`$formatNumber(-5.5,"0.00\ud800(0.00)",{"pattern-separator":"\ud800"})`, `null`, `"(5.50)"`},
		{`$formatNumber(0.42,"0\ud800",{"percent":"\ud800"})`, `null`, `"42\ud800"`},
		{`$formatNumber(0.042,"00\udfff",{"per-mille":"\udfff"})`, `null`, `"42\udfff"`},
		{`$formatNumber(52,"0\ud8000",{"exponent-separator":"\ud800"})`, `null`, `"5\ud8001"`},
		{`$formatNumber(12345,"#\ud800##0",{"grouping-separator":"\ud800"})`, `null`, `"12\ud800345"`},
		{`$formatNumber(42,"\ud8000",{"digit":"\ud800"})`, `null`, `"42"`},
		{`$formatNumber(42,"\ud800\ud800",{"zero-digit":"\ud800"})`, `null`, `"\ud804\ud802"`},
		{`$formatNumber(42,"\udfff\udfff",{"zero-digit":"\udfff"})`, `null`, "\"\ue003\ue001\""},
		{`$formatNumber(42,"0\ud800#")`, `null`, "ERR D3090"},
		{`$formatNumber(4,"\uffff",{"zero-digit":"\uffff"})`, `null`, `"\u0003"`},
		{`$formatNumber(-4,"\udc000",{"minus-sign":"\ud800"})`, `null`, `"𐀀4"`},
		{`$formatNumber(42.5,picture,locale)`, `{"picture":"0\ud8000","locale":{"decimal-separator":"\ud800"}}`, `"42\ud8005"`},
	})
}

func TestUTF16NumberPictureOutputLimitCountsPairsOnce(t *testing.T) {
	const expected = "𐀀4"
	picture := "\xed\xb0\x800"                             // U+DC00 followed by the digit placeholder.
	locale := map[string]any{"minus-sign": "\xed\xa0\x80"} // U+D800.
	for _, limit := range []int{len(expected), len(expected) - 1} {
		b, err := budget.New(context.Background(), budget.Limits{MaxOutputBytes: limit})
		if err != nil {
			t.Fatal(err)
		}
		got, err := formatNumber(&CallContext{Budget: b}, -4, picture, locale)
		b.Close()
		if limit == len(expected) {
			if err != nil || got != expected {
				t.Fatalf("got %q, %v, want %q", got, err, expected)
			}
		} else if !errors.Is(err, budget.ErrLimit) {
			t.Fatalf("got %q, %v, want a limit error", got, err)
		}
	}
}

func TestReferenceUTF16MultiUnitNumberSymbols(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatNumber(1.5,"0\ud8000",{"decimal-separator":"\ud800x"})`, `null`, "ERR D3086"},
		{`$formatNumber(1.5,"0\ud8000",{"grouping-separator":"\ud800x"})`, `null`, "ERR D3086"},
		{`$formatNumber(52,"0\ud8000",{"exponent-separator":"\ud800x"})`, `null`, "ERR D3086"},
		{`$formatNumber(1,"#\ud800A0",{"digit":"\ud800A"})`, `null`, `"#\ud800A1"`},
		{`$formatNumber(1234.5,"0\ud8000\ud800A",{"decimal-separator":"\ud800A"})`, `null`, "ERR D3086"},
		{`$formatNumber(1,"0",{"decimal-separator":"\ud800x"})`, `null`, `"1\ud800"`},
		{`$formatNumber(1,"0\ud800A",{"decimal-separator":"\ud800A"})`, `null`, `"1\ud800A\ud800A"`},
		{`$formatNumber(1,"\ud800\ud800\ud800",{"zero-digit":"\ud800A"})`, `null`, `"\ud800A\ud800A\ud801"`},
	})
}
