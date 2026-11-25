package jsonata

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

func TestReferenceUTF16IntegerPictures(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatInteger(1234,"#\ud800##0")`, `null`, `"1\ud800234"`},
		{`$formatInteger(1234,"#\udfff##0")`, `null`, `"1\udfff234"`},
		{`$formatInteger(1234,p)`, `{"p":"#\ud801😀##0"}`, `"1\ud801😀234"`},
		{`$formatInteger(42,"0\ud800;o")`, `null`, `"42\ud800th"`},
		{`$formatInteger(42,"#\ud800##\udfff00")`, `null`, `"𐏿42"`},
		{`$parseInteger($,"#\ud800##0")`, `"1\ud800234"`, `1`},
		{`$parseInteger($,"#\ud800##0")`, `"-1\ud800234"`, `-1`},
		{`$parseInteger($,"#\ud800##0")`, `"1\ud800234"`, `1234`},
		{`$parseInteger($,"#\ud800##0")`, `"1\udfff234"`, `1`},
		{`$formatInteger(42,"٠\ud8000")`, `null`, "ERR D3131"},
		{`$parseInteger($,"\ud800")`, `"123"`, "ERR D3130"},
	})
}

func TestReferenceUTF16IntegerPictureFamiliesAndRange(t *testing.T) {
	checkReferenceCases(t, []referenceCase{
		{`$formatInteger(42,"𝟘\ud8000")`, `null`, `"4𝟘\ud8002"`},
		{`$formatInteger(42,"𝟘\ud800")`, `null`, "ERR D3130"},
		{`$formatInteger(1e21,"٠\ud800٠")`, `null`, "\"١\\ud800ڕ\\ud800ٛ\\ud800٢\\ud800١\""},
		{`$exists($parseInteger("1" & $pad("",500,"0"),"0\ud800000"))`, `null`, `true`},
		{`$boolean($parseInteger("1" & $pad("",500,"0"),"0\ud800000"))`, `null`, "ERR D1001"},
		{`$exists($parseInteger("x\ud800","0\ud800000"))`, `null`, `true`},
	})
}

func TestReferenceParseIntegerOverflow(t *testing.T) {
	const positive = `$parseInteger("1" & $pad("",500,"0"),"0")`
	const negative = `$parseInteger("-1" & $pad("",500,"0"),"0")`
	const surrogate = `$parseInteger("1" & $pad("",500,"0"),"0\ud800000")`
	const negativeHex = `$parseInteger("-0X" & $pad("",256,"F"),"0")`
	checkReferenceCases(t, []referenceCase{
		{positive, `null`, `null`},
		{negative, `null`, `null`},
		{surrogate, `null`, `null`},
		{`{"numbers":[` + positive + `,` + negative + `],"surrogate":` + surrogate + `}`, `null`, `{"numbers":[null,null],"surrogate":null}`},
		{positive + ` > 0`, `null`, `true`},
		{negative + ` < 0`, `null`, `true`},
		{positive + ` = null`, `null`, `false`},
		{`$parseInteger("0,x" & $pad("",300,"f"),"#,##0")`, `null`, `null`},
		{negativeHex, `null`, `null`},
		{negativeHex + ` < 0`, `null`, `true`},
		{`$parseInteger("0x" & $pad("",300,"0") & "2A","0")`, `null`, `42`},
		{`$parseInteger("0x" & $pad("",13,"F") & "8" & $pad("",242,"0"),"0")`, `null`, `1.7976931348623157e+308`},
		{`$parseInteger("0x" & $pad("",13,"F") & "C" & $pad("",242,"0"),"0")`, `null`, `null`},
		{`[1/0,0/0]`, `null`, `[null,null]`},
		{`$string(` + positive + `)`, `null`, "ERR D3001"},
		{`$parseInteger("123x" & $pad("",500,"0"),"0")`, `null`, `123`},
	})

	ctx := context.Background()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	program, err := engine.CompileRaw(ctx, negative)
	if err != nil {
		t.Fatal(err)
	}
	value, err := program.EvalRaw(ctx, nil, jsontrans.EvalOptions{})
	if n, ok := value.(float64); err != nil || !ok || !math.IsInf(n, -1) {
		t.Fatalf("raw overflow = %v, %v, want -Inf", value, err)
	}

	program, err = engine.CompileRaw(ctx, `$parseInteger($,"0")`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`"-1` + strings.Repeat("0", 500) + `"`)
	document, err := jsontrans.PrepareJSON(ctx, input, jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{3, 4} {
		options := jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxOutputBytes: limit}}
		result, err := jsontrans.EvalPreparedJSON(ctx, program, document, options)
		if limit == 3 {
			if !errors.Is(err, jsontrans.ErrLimit) {
				t.Fatalf("3-byte limit: got %+v, %v, want limit error", result, err)
			}
		} else if err != nil || !result.Defined || string(result.Value) != "null" {
			t.Fatalf("4-byte limit: got %+v, %v, want defined null", result, err)
		}
	}
}
