package jsonata

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestCoreExpressions(t *testing.T) {
	cases := []struct {
		expr        string
		input, want any
		defined     bool
	}{
		{`1 + 2 * 3`, nil, float64(7), true},
		{`foo.bar`, map[string]any{"foo": map[string]any{"bar": "ok"}}, "ok", true},
		{`missing`, map[string]any{}, nil, false},
		{`[1, 2, 3][1]`, nil, float64(2), true},
		{`true ? "yes" : "no"`, nil, "yes", true},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			e, err := New()
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.Compile(context.Background(), tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Eval(context.Background(), tc.input, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Defined != tc.defined || !reflect.DeepEqual(got.Value, tc.want) {
				t.Fatalf("got %#v, want %#v (defined %v)", got, tc.want, tc.defined)
			}
		})
	}
}

func TestLexicalFunctions(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `($x := 3; $f := function($y) {$x + $y}; $f(4))`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != float64(7) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestWildcardAndComparison(t *testing.T) {
	e, _ := New()
	for _, tc := range []struct {
		expr string
		want any
	}{{`[1..3]`, []any{float64(1), float64(2), float64(3)}}, {`"a" < "b"`, true}, {`foo.*`, []any{float64(1), float64(2)}}} {
		p, err := e.Compile(context.Background(), tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.Eval(context.Background(), map[string]any{"foo": map[string]any{"a": 1, "b": 2}}, jsontrans.EvalOptions{})
		if err != nil || !reflect.DeepEqual(got.Value, tc.want) {
			t.Fatalf("%s got %#v, %v", tc.expr, got, err)
		}
	}
}

func TestNestedEvalSharesContext(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `$eval("a + 1", {"a": 4})`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != float64(5) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestPathFlattensNestedArrays(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `items.price`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), map[string]any{"items": []any{[]any{map[string]any{"price": 1}}, map[string]any{"price": 2}}}, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, []any{float64(1), float64(2)}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestComments(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `1 /* note */ + 2`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != float64(3) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestStringConcatenationNumberFormatting(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `"total: " & (0.1 + 0.2)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != "total: 0.3" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func FuzzCompileAndEval(f *testing.F) {
	for _, s := range []string{`foo.bar`, `[1..3]`, `($x := 1; $x + 2)`, `"a"`, `/* comment */ 2`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		e, err := New(WithLimits(jsontrans.Limits{MaxExpressionBytes: 4096, MaxSteps: 10000, MaxDepth: 64, MaxCollectionItems: 1000}))
		if err != nil {
			t.Fatal(err)
		}
		p, err := e.Compile(context.Background(), expr)
		if err != nil {
			return
		}
		_, _ = p.Eval(context.Background(), map[string]any{"foo": map[string]any{"bar": float64(1)}}, jsontrans.EvalOptions{})
	})
}

func TestMembership(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `2 in [1, 2, 3]`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != true {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestRegexLiteral(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `$contains("hello", /ell/)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != true {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestSortStage(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `items.price^(>$)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), map[string]any{"items": []any{map[string]any{"price": 1}, map[string]any{"price": 3}, map[string]any{"price": 2}}}, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, []any{float64(3), float64(2), float64(1)}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestLimitsProvider(t *testing.T) {
	limits := jsontrans.Limits{MaxSteps: 123}
	e, err := New(WithLimits(limits))
	if err != nil {
		t.Fatal(err)
	}
	if e.Limits().MaxSteps != 123 {
		t.Fatalf("engine limits: %#v", e.Limits())
	}
	p, err := e.CompileRaw(context.Background(), `1`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Limits().MaxSteps != 123 {
		t.Fatalf("program limits: %#v", p.Limits())
	}
}

func TestGroupingStage(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `items{category: $sum(price)}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"items": []any{map[string]any{"category": "a", "price": 1}, map[string]any{"category": "a", "price": 2}, map[string]any{"category": "b", "price": 4}}}
	got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	want := map[string]any{"a": float64(3), "b": float64(4)}
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestPathIndexBinding(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `items#$i.{"index": $i, "value": value}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"items": []any{map[string]any{"value": "a"}, map[string]any{"value": "b"}}}
	want := []any{map[string]any{"index": float64(0), "value": "a"}, map[string]any{"index": float64(1), "value": "b"}}
	got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestKeepAndMultiIndex(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `$[[0,2]][]`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), []any{1, 2, 3}, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, []any{float64(1), float64(3)}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestLambdaSignature(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `function($x)<n:n>{$x + 1}(2)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != float64(3) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestUnicodeNamesAndLambda(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `λ($x){$x.имя}({"имя":"да"})`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != "да" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestQuotedPathSelector(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `account."full name"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), map[string]any{"account": map[string]any{"full name": "Ada"}}, jsontrans.EvalOptions{})
	if err != nil || got.Value != "Ada" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestChainApplication(t *testing.T) {
	e, _ := New()
	for _, expr := range []string{`"hello" ~> $uppercase()`, `"hello" ~> $uppercase`, `($f := $uppercase ~> $lowercase; $f("Hello"))`} {
		p, err := e.Compile(context.Background(), expr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
		want := "HELLO"
		if strings.Contains(expr, "$f :=") {
			want = "hello"
		}
		if err != nil || got.Value != want {
			t.Fatalf("%s got %#v, %v", expr, got, err)
		}
	}
}

func TestTransform(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `$ ~> |items|{"price": price * 2}, "old"|`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"items": []any{map[string]any{"price": 2, "old": true}}}
	want := map[string]any{"items": []any{map[string]any{"price": float64(4)}}}
	got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
	if !reflect.DeepEqual(input, map[string]any{"items": []any{map[string]any{"price": 2, "old": true}}}) {
		t.Fatal("input mutated")
	}
}

func TestPathFlattensSelectedArrays(t *testing.T) {
	e, _ := New()
	for _, expr := range []string{`items.value`, `$.value`} {
		p, err := e.Compile(context.Background(), expr)
		if err != nil {
			t.Fatal(err)
		}
		data := []any{map[string]any{"value": []any{1, 2}}, map[string]any{"value": []any{3, 4}}}
		var input any = data
		if expr == `items.value` {
			input = map[string]any{"items": data}
		}
		got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
		if err != nil || !reflect.DeepEqual(got.Value, []any{float64(1), float64(2), float64(3), float64(4)}) {
			t.Fatalf("%s got %#v, %v", expr, got, err)
		}
	}
}

func TestRangeNativeErrors(t *testing.T) {
	e, _ := New()
	for _, tc := range []struct{ expr, code string }{{`[false..$missing]`, "T2003"}, {`[$missing..true]`, "T2004"}, {`[0..10000000]`, "D2014"}} {
		p, err := e.Compile(context.Background(), tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != tc.code {
			t.Fatalf("%s: %v", tc.expr, err)
		}
	}
}

func TestPartialApplication(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `($f := $substring(?, 1, 2); $f("hello"))`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != "el" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestTailRecursion(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `($f := function($n, $acc){$n = 0 ? $acc : $f($n - 1, $acc + 1)}; $f(1000, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != float64(1000) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestArithmeticNativeErrors(t *testing.T) {
	e, _ := New()
	for _, tc := range []struct{ expr, code string }{{`"s" - 1`, "T2001"}, {`1 + null`, "T2002"}, {`-"s"`, "D1002"}, {`3(?)`, "T1008"}} {
		p, err := e.Compile(context.Background(), tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != tc.code {
			t.Fatalf("%s: %v", tc.expr, err)
		}
	}
}

func TestUndefinedRawInput(t *testing.T) {
	e, _ := New()
	p, err := e.CompileRaw(context.Background(), `$`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.EvalRaw(context.Background(), Undefined, jsontrans.EvalOptions{})
	if err != nil || !IsUndefined(got) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestRejectOverflowAndSnapshotEngine(t *testing.T) {
	var captured *Engine
	e, err := New(func(x *Engine) error { captured = x; return nil })
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(context.Background(), `1`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), map[string]any{"bad": json.Number("1e999")}, jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("overflow input: %v", err)
	}
	before := p.Limits()
	captured.limits.MaxSteps = 2
	if p.Limits() != before {
		t.Fatal("program limits changed after compile")
	}
}

func TestRejectCyclicHostResult(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	e, err := New(WithFunction("cycle", NativeFunction{Fn: func(*CallContext, []Value) (Value, error) { return cycle, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(context.Background(), `$cycle()`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if !errors.Is(err, jsontrans.ErrConversion) {
		t.Fatalf("got %v", err)
	}
}

func TestJoinRetainsParentFocus(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `employees@$e.contacts[id=$e.id].{"name":$e.name,"phone":phone}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"employees": []any{map[string]any{"id": 1, "name": "Ada"}}, "contacts": []any{map[string]any{"id": 1, "phone": "123"}}}
	got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	want := map[string]any{"name": "Ada", "phone": "123"}
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestParentReference(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `order.items.{"name": name, "order": %.id}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"order": map[string]any{"id": "x", "items": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}}}}
	want := []any{map[string]any{"name": "a", "order": "x"}, map[string]any{"name": "b", "order": "x"}}
	got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestConstructedObjectOrder(t *testing.T) {
	e, err := New(WithFunction("keyorder", NativeFunction{N: 1, Fn: func(c *CallContext, args []Value) (Value, error) {
		keys := c.ObjectKeys(args[0])
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return out, nil
	}}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(context.Background(), `$keyorder({"z": 1, "a": 2})`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, []any{"z", "a"}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestCommonEvalRejectsUndefinedInput(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `1`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), Undefined, jsontrans.EvalOptions{})
	if !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}

func TestMutualRecursion(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `($even := function($n){$n=0 ? true : $odd($n-1)}; $odd := function($n){$n=0 ? false : $even($n-1)}; $even(1000))`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != true {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestArrayConstructorFlattensSelectedArrays(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `[a, [b]]`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), map[string]any{"a": []any{1, 2}, "b": []any{3, 4}}, jsontrans.EvalOptions{})
	want := []any{float64(1), float64(2), []any{float64(3), float64(4)}}
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestKeepFlagPersistsAcrossPath(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `items[][value=1].value`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), map[string]any{"items": []any{map[string]any{"value": 1}, map[string]any{"value": 2}}}, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, []any{float64(1)}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestLiteralNewline(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "\"a\nb\"")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != "a\nb" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestCyclicHostResultInExpression(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	e, _ := New(WithFunction("cycle", NativeFunction{Fn: func(*CallContext, []Value) (Value, error) { return cycle, nil }}))
	for _, expr := range []string{`$cycle() & "x"`, `$cycle() = {}`, `$cycle() ~> |$|{}|`} {
		p, err := e.Compile(context.Background(), expr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{}); err == nil {
			t.Fatalf("%s unexpectedly succeeded", expr)
		}
	}
}

func TestGeneratedSharedArraysRespectBudget(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`($a := [false];`)
	for i := 0; i < 20; i++ {
		builder.WriteString(`$a := [[$a],[$a]];`)
	}
	for _, suffix := range []string{`$a and true)`, `$a.foo)`} {
		e, err := New(WithLimits(jsontrans.Limits{MaxSteps: 1000}))
		if err != nil {
			t.Fatal(err)
		}
		p, err := e.Compile(context.Background(), builder.String()+suffix)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
		if !errors.Is(err, jsontrans.ErrLimit) {
			t.Fatalf("%s: %v", suffix, err)
		}
	}
}

func TestArrayConstructionChecksCollectionBeforeAppend(t *testing.T) {
	expr := `($a := [1..10]; [$a,$a])`
	e, err := New(WithLimits(jsontrans.Limits{MaxCollectionItems: 10}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(context.Background(), expr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestNestedEvalWithParent(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `$eval("a.b.%")`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}, "x": 2}}, jsontrans.EvalOptions{})
	want := map[string]any{"b": map[string]any{"c": float64(1)}, "x": float64(2)}
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestNativeErrorPositionsUTF16(t *testing.T) {
	e, _ := New()
	for _, tc := range []struct {
		expr, code string
		pos        int
	}{{`$sqrt(-1)`, "D3060", 6}, {`$length(1)`, "T0410", 8}, {`"😀"+1`, "T2001", 5}} {
		p, err := e.Compile(context.Background(), tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Code != tc.code || native.Position != tc.pos {
			t.Fatalf("%s: %#v (%v)", tc.expr, native, err)
		}
	}
}

func TestStringEscapesAndUnpairedSurrogate(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `'a\nb'`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || got.Value != "a\nb" {
		t.Fatalf("got %#v, %v", got, err)
	}
	p, err = e.Compile(context.Background(), `$encodeUrl('\uD800')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	var native *jsontrans.Error
	if !errors.As(err, &native) || native.Code != "D3140" {
		t.Fatalf("got %v", err)
	}
}

func TestJoinIndexIsLocalToParent(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `groups@$g.items@$item#$i.{"g":$g.id,"i":$i}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"groups": []any{map[string]any{"id": 1}, map[string]any{"id": 2}}, "items": []any{"a", "b"}}
	want := []any{map[string]any{"g": float64(1), "i": float64(0)}, map[string]any{"g": float64(1), "i": float64(1)}, map[string]any{"g": float64(2), "i": float64(0)}, map[string]any{"g": float64(2), "i": float64(1)}}
	got, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got.Value, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestUTF16ObjectInputAndBindings(t *testing.T) {
	key := string([]byte{0xed, 0xa0, 0x80})
	input := jsontrans.UTF16Object{key: "ok"}
	e, _ := New()
	p, err := e.CompileRaw(context.Background(), `$`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), input, jsontrans.EvalOptions{Bindings: map[string]any{key: "bound"}})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(map[string]any)
	if !ok || m[key] != "ok" {
		t.Fatalf("got %#v", v)
	}
}

func TestDuplicateObjectKey(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), `{"a":1,"a":2}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	var native *jsontrans.Error
	if !errors.As(err, &native) || native.Code != "D1009" {
		t.Fatalf("got %v", err)
	}
}

func TestEvaluationDepthOption(t *testing.T) {
	e, err := New(WithEvaluationDepth(12))
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(context.Background(), `($f:=function($x){$x=0 ? 1 : $x*$f($x-1)};$f(20))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	var native *jsontrans.Error
	if !errors.As(err, &native) || native.Code != "U1001" || !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestSurrogateConcatenationCanonicalization(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		expr string
		want any
	}{
		{`("\uD83D" & "\uDE00") = "😀"`, true},
		{`$distinct(["\uD83D" & "\uDE00","😀"])`, []any{"😀"}},
		{`$lookup({("\uD83D" & "\uDE00"):1}, "😀")`, float64(1)},
	} {
		p, err := e.Compile(context.Background(), tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
		if err != nil || !reflect.DeepEqual(got.Value, tc.want) {
			t.Errorf("%s: got %#v, %v; want %#v", tc.expr, got, err, tc.want)
		}
	}
}

func TestDescendantAndTransformDepthLimits(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{
		`($f:=function($n,$x){$n=0?$x:$f($n-1,{"a":$x})};$count($f(1000,0).**))`,
		`{} ~> |($f:=function($n,$x){$n=0?$x:$f($n-1,[[$x]])};$f(100,false))|{}|`,
	} {
		p, err := e.Compile(context.Background(), expr)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxDepth: 20}})
		if !errors.Is(err, jsontrans.ErrLimit) {
			t.Errorf("%s: expected depth limit, got %v", expr, err)
		}
	}
}

func TestInvokePreservesCapturedJSONInputOrder(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	ctx := jsonvalue.WithObjectOrder(base)
	b, err := budget.New(ctx, jsontrans.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Decode([]byte(`{"b":1,"a":2}`), b)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := New()
	p, err := e.CompileRaw(ctx, `function(){$keys($)}`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(ctx, input, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	cancel()
	got, err := Invoke(context.Background(), v.(Callable), nil, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got, Sequence{"b", "a"}) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestSignatureErrorPosition(t *testing.T) {
	e, _ := New()
	_, err := e.Compile(context.Background(), `function($x)<n<n>:n>{$x}`)
	var native *jsontrans.Error
	if !errors.As(err, &native) || native.Code != "S0401" || native.Position != 15 {
		t.Fatalf("got %#v, %v", native, err)
	}
}
