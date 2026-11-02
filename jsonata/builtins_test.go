package jsonata

import (
	"context"
	"errors"
	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinCoreSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []Value
		want Value
	}{
		{"sum", []Value{[]any{float64(1), float64(2)}}, float64(3)},
		{"average", []Value{[]any{}}, Undefined},
		{"substring", []Value{"hello", float64(-3), float64(2)}, "ll"},
		{"boolean", []Value{[]any{float64(0), "x"}}, true},
		{"round", []Value{float64(2.5)}, float64(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := builtins()[tt.name]
			if f == nil {
				t.Fatalf("missing builtin")
			}
			got, err := f.Call(&CallContext{}, tt.args)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, %v; want %#v", got, err, tt.want)
			}
		})
	}
}

func TestBuiltinMapCallbackReceivesItemAndIndex(t *testing.T) {
	f := NativeFunction{N: 2, Fn: func(_ *CallContext, a []Value) (Value, error) { return a[0].(float64) + a[1].(float64), nil }}
	c := &CallContext{Invoke: func(f Callable, a []Value) (Value, error) { return f.Call(&CallContext{}, a) }}
	got, err := builtins()["map"].Call(c, []Value{[]any{float64(10), float64(20)}, f})
	if err != nil || !reflect.DeepEqual(got, Sequence{float64(10), float64(21)}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinSplitAndPad(t *testing.T) {
	tests := []struct {
		name string
		args []Value
		want Value
	}{
		{"split", []Value{"a,b,c", ",", float64(2)}, []any{"a", "b"}},
		{"pad", []Value{"x", float64(-4), "ab"}, "abax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := builtins()[tt.name]
			if fn == nil {
				t.Fatal("missing builtin")
			}
			got, err := fn.Call(&CallContext{}, tt.args)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, %v", got, err)
			}
		})
	}
}

func TestBuiltinAggregationRejectsWrongArity(t *testing.T) {
	for _, name := range []string{"sum", "count", "average"} {
		for _, a := range [][]Value{nil, {[]any{}, []any{}}} {
			_, err := builtins()[name].Call(&CallContext{}, a)
			if err == nil {
				t.Errorf("%s(%v): expected error", name, a)
			}
		}
	}
}

func TestBuiltinSplitHonorsCollectionLimit(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxCollectionItems: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, err = builtins()["split"].Call(&CallContext{Budget: b}, []Value{"a,b,c", ","})
	if err == nil {
		t.Fatal("expected collection limit error")
	}
}

func TestBuiltinFormatBase(t *testing.T) {
	got, err := builtins()["formatBase"].Call(&CallContext{}, []Value{float64(255), float64(16)})
	if err != nil || got != "ff" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinSpreadObject(t *testing.T) {
	got, err := builtins()["spread"].Call(&CallContext{}, []Value{map[string]any{"a": float64(1), "b": float64(2)}})
	want := Sequence{map[string]any{"a": float64(1)}, map[string]any{"b": float64(2)}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinFormatIntegerDecimal(t *testing.T) {
	got, err := builtins()["formatInteger"].Call(&CallContext{}, []Value{float64(1234), "#,##0;o"})
	if err != nil || got != "1,234th" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinIntegerWords(t *testing.T) {
	f := builtins()["formatInteger"]
	got, err := f.Call(&CallContext{}, []Value{float64(3731), "w;o"})
	if err != nil || got != "three thousand, seven hundred and thirty-first" {
		t.Fatalf("format got %#v, %v", got, err)
	}
	got, err = builtins()["parseInteger"].Call(&CallContext{}, []Value{got, "w;o"})
	if err != nil || got != float64(3731) {
		t.Fatalf("parse got %#v, %v", got, err)
	}
}

func TestBuiltinFormatNumberPicture(t *testing.T) {
	got, err := builtins()["formatNumber"].Call(&CallContext{}, []Value{float64(12345.6), "#,###.00"})
	if err != nil || got != "12,345.60" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinFromMillisPicture(t *testing.T) {
	got, err := builtins()["fromMillis"].Call(&CallContext{}, []Value{float64(1521801216617), "[Y0001]-[M01]-[D01]"})
	if err != nil || got != "2018-03-23" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinToMillisPicture(t *testing.T) {
	got, err := builtins()["toMillis"].Call(&CallContext{}, []Value{"2018-03-27", "[Y1]-[M01]-[D01]"})
	if err != nil || got != float64(1522108800000) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinSortDoesNotMutateInput(t *testing.T) {
	input := []any{float64(3), float64(1), float64(2)}
	got, err := builtins()["sort"].Call(&CallContext{}, []Value{input})
	if err != nil || !reflect.DeepEqual(got, []any{float64(1), float64(2), float64(3)}) || input[0] != float64(3) {
		t.Fatalf("got %#v, input %#v, %v", got, input, err)
	}
}

func TestBuiltinDistinctPreservesExplicitArray(t *testing.T) {
	got, err := builtins()["distinct"].Call(&CallContext{}, []Value{[]any{float64(1), float64(1)}})
	if err != nil || !reflect.DeepEqual(got, []any{float64(1)}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}
func TestBuiltinEncodeURLPreservesReserved(t *testing.T) {
	got, err := builtins()["encodeUrl"].Call(&CallContext{}, []Value{"https://a.example/?q=x y"})
	if err != nil || got != "https://a.example/?q=x%20y" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinRoundDecimalTie(t *testing.T) {
	for _, v := range []float64{4.525, 4.515} {
		got, err := builtins()["round"].Call(&CallContext{}, []Value{v, float64(2)})
		if err != nil || got != 4.52 {
			t.Fatalf("%v got %#v, %v", v, got, err)
		}
	}
}

func TestBuiltinStringNumericPrecisionAndPretty(t *testing.T) {
	f := builtins()["string"]
	got, err := f.Call(&CallContext{}, []Value{float64(22) / 7})
	if err != nil || got != "3.14285714285714" {
		t.Fatalf("got %#v, %v", got, err)
	}
	got, err = f.Call(&CallContext{}, []Value{[]any{"x", float64(1)}, true})
	if err != nil || got != "[\n  \"x\",\n  1\n]" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinEvalUsesFocusWhenOptionalArgumentUndefined(t *testing.T) {
	c := &CallContext{Focus: float64(7), Evaluate: func(_ string, v Value) (Value, error) { return v, nil }}
	got, err := builtins()["eval"].Call(c, []Value{"$", Undefined})
	if err != nil || got != float64(7) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinCloneDoesNotShareNestedObjects(t *testing.T) {
	input := map[string]any{"nested": map[string]any{"value": float64(1)}}
	got, err := builtins()["clone"].Call(&CallContext{}, []Value{input})
	if err != nil {
		t.Fatal(err)
	}
	got.(map[string]any)["nested"].(map[string]any)["value"] = float64(2)
	if input["nested"].(map[string]any)["value"] != float64(1) {
		t.Fatal("input mutated")
	}
}

func TestBuiltinOutputLimit(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxOutputBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for _, tt := range []struct {
		name string
		args []Value
	}{{"join", []Value{[]any{"abc", "def"}, ""}}, {"pad", []Value{"x", float64(5)}}, {"base64encode", []Value{"abcd"}}} {
		_, err := builtins()[tt.name].Call(&CallContext{Budget: b}, tt.args)
		if !errors.Is(err, budget.ErrLimit) {
			t.Errorf("%s got %v", tt.name, err)
		}
	}
}
func TestBuiltinFormatBaseBeyondInt64(t *testing.T) {
	got, err := builtins()["formatBase"].Call(&CallContext{}, []Value{1e20, float64(10)})
	if err != nil || got != "100000000000000000000" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinPictureAndStringOutputLimits(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxOutputBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	c := &CallContext{Budget: b}
	for _, tt := range []struct {
		name string
		args []Value
	}{{"formatNumber", []Value{float64(12), "00000"}}, {"fromMillis", []Value{float64(0), "[Y0001]-[M01]"}}, {"string", []Value{[]any{"12345"}}}} {
		_, err := builtins()[tt.name].Call(c, tt.args)
		if !errors.Is(err, budget.ErrLimit) {
			t.Errorf("%s got %v", tt.name, err)
		}
	}
}

func TestBuiltinFormatBaseNonFinite(t *testing.T) {
	for _, tt := range []struct {
		value float64
		want  string
	}{{math.Inf(1), "Infinity"}, {math.Inf(-1), "-Infinity"}, {math.NaN(), "NaN"}} {
		got, err := builtins()["formatBase"].Call(&CallContext{}, []Value{tt.value})
		if err != nil || got != tt.want {
			t.Errorf("got %#v, %v want %s", got, err, tt.want)
		}
	}
}

func TestBuiltinCollectionLimits(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxCollectionItems: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	c := &CallContext{Budget: b}
	for _, tt := range []struct {
		name string
		args []Value
	}{{"append", []Value{[]any{1., 2.}, []any{3.}}}, {"reverse", []Value{[]any{1., 2., 3.}}}, {"join", []Value{[]any{"a", "b", "c"}}}} {
		_, err := builtins()[tt.name].Call(c, tt.args)
		if !errors.Is(err, budget.ErrLimit) {
			t.Errorf("%s got %v", tt.name, err)
		}
	}
}

func TestBuiltinStringRespectsObjectOrder(t *testing.T) {
	obj := map[string]any{"first": float64(1), "second": float64(2)}
	c := &CallContext{ObjectKeys: func(v Value) []string { return []string{"second", "first"} }}
	got, err := builtins()["string"].Call(c, []Value{obj, true})
	if err != nil || got != "{\n  \"second\": 2,\n  \"first\": 1\n}" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinDistinctChargesNestedComparisons(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxSteps: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	nested := []any{1., 2., 3., 4., 5., 6., 7., 8., 9., 10.}
	_, err = builtins()["distinct"].Call(&CallContext{Budget: b}, []Value{[]any{nested, nested}})
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestBuiltinStringDoesNotEscapeHTML(t *testing.T) {
	got, err := builtins()["string"].Call(&CallContext{}, []Value{map[string]any{"value": "<>&"}})
	if err != nil || got != "{\"value\":\"<>&\"}" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinRoundInfinity(t *testing.T) {
	got, err := builtins()["round"].Call(&CallContext{}, []Value{math.Inf(1), float64(2)})
	if err != nil || !math.IsInf(got.(float64), 1) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinSumPreservesOverflowResult(t *testing.T) {
	got, err := builtins()["sum"].Call(&CallContext{}, []Value{[]any{1e308, 1e308}})
	if err != nil || !math.IsInf(got.(float64), 1) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinFormatBaseLargeHexMatchesReference(t *testing.T) {
	got, err := builtins()["formatBase"].Call(&CallContext{}, []Value{1e20, float64(16)})
	if err != nil || got != "56bc75e2d63100000" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinStringPreservesUnicodeSeparators(t *testing.T) {
	got, err := builtins()["string"].Call(&CallContext{}, []Value{[]any{"x\u2028y\u2029z"}})
	if err != nil || got != "[\"x\u2028y\u2029z\"]" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinBase64UsesReferenceBinaryStrings(t *testing.T) {
	f := builtins()
	got, err := f["base64encode"].Call(&CallContext{}, []Value{"é€"})
	if err != nil || got != "6aw=" {
		t.Fatalf("encode got %#v, %v", got, err)
	}
	got, err = f["base64decode"].Call(&CallContext{}, []Value{"6Q=="})
	if err != nil || got != "é" {
		t.Fatalf("decode got %#v, %v", got, err)
	}
}

func TestBuiltinBooleanChargesNestedDAG(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxSteps: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var value any = []any{false}
	for i := 0; i < 12; i++ {
		value = []any{value, value}
	}
	_, err = builtins()["boolean"].Call(&CallContext{Budget: b}, []Value{value})
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestBuiltinBooleanRejectsCycle(t *testing.T) {
	value := make([]any, 1)
	value[0] = value
	_, err := builtins()["boolean"].Call(&CallContext{}, []Value{value})
	if !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}

func TestBuiltinUnicodeFullCaseMapping(t *testing.T) {
	tests := []struct{ name, input, want string }{{"lowercase", "İ", "i\u0307"}, {"lowercase", "ΟΣ", "ος"}, {"uppercase", "straße", "STRASSE"}}
	for _, tt := range tests {
		got, err := builtins()[tt.name].Call(&CallContext{}, []Value{tt.input})
		if err != nil || got != tt.want {
			t.Errorf("%s(%q) got %#v, %v", tt.name, tt.input, got, err)
		}
	}
}

func TestBuiltinUnicodeCaseAcrossChunk(t *testing.T) {
	input := strings.Repeat("A", 4095) + "ΟΣ"
	want := strings.Repeat("a", 4095) + "ος"
	got, err := builtins()["lowercase"].Call(&CallContext{}, []Value{input})
	if err != nil || got != want {
		t.Fatalf("incorrect result or error: %v", err)
	}
}

func TestBuiltinDatePictureWidthHonorsLimit(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxOutputBytes: 100, MaxCollectionItems: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, err = builtins()["fromMillis"].Call(&CallContext{Budget: b}, []Value{float64(0), "[Y,1000000000]"})
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestBuiltinStringQuotesLoneSurrogate(t *testing.T) {
	s := string([]byte{0xed, 0xa0, 0xbd})
	got, err := builtins()["string"].Call(&CallContext{}, []Value{[]any{s}})
	if err != nil || got != `["\ud83d"]` {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinCloneUsesJSONataStringConversion(t *testing.T) {
	fn := NativeFunction{N: 0, Fn: func(*CallContext, []Value) (Value, error) { return nil, nil }}
	value := map[string]any{"f": fn, "missing": Undefined, "array": []any{Undefined}, "number": float64(22) / 7}
	got, err := builtins()["clone"].Call(&CallContext{}, []Value{value})
	want := map[string]any{"f": "", "array": []any{nil}, "number": 3.14285714285714}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinBase64DecodeReferencePermissiveInput(t *testing.T) {
	for _, tt := range []struct{ input, want string }{{"YQ", "a"}, {"Y Q==", "a"}, {"__8=", "ÿÿ"}, {"###", ""}} {
		got, err := builtins()["base64decode"].Call(&CallContext{}, []Value{tt.input})
		if err != nil || got != tt.want {
			t.Errorf("%q got %#v, %v", tt.input, got, err)
		}
	}
}

func TestBuiltinSubstringInfiniteStart(t *testing.T) {
	f := builtins()["substring"]
	for _, tt := range []struct {
		start float64
		want  string
	}{{math.Inf(1), ""}, {math.Inf(-1), "foo"}} {
		got, err := f.Call(&CallContext{}, []Value{"foo", tt.start})
		if err != nil || got != tt.want {
			t.Errorf("got %#v, %v", got, err)
		}
	}
}

func TestBuiltinSubstringNaNLength(t *testing.T) {
	got, err := builtins()["substring"].Call(&CallContext{}, []Value{"foo", float64(1), math.NaN()})
	if err != nil || got != "" {
		t.Fatalf("got %#v, %v", got, err)
	}
}
