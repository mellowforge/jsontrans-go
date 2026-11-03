package jsonata

import (
	"context"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

func TestEnvironmentLookupPrecedence(t *testing.T) {
	base := &environment{builtins: map[string]Callable{"sum": nil}, functions: map[string]Callable{"custom": nil}}
	local := map[string]Value{"sum": Undefined}
	if value, ok := base.lookup(local, "sum"); !ok || !IsUndefined(value) {
		t.Fatal("local undefined must shadow builtin")
	}
	if _, ok := base.lookup(local, "custom"); !ok {
		t.Fatal("custom function missing from parent")
	}
	if _, ok := base.lookup(local, "missing"); ok {
		t.Fatal("missing name found")
	}
}

func TestEnvironmentLexicalSemantics(t *testing.T) {
	e, err := New(WithFunction("sum", NativeFunction{Fn: func(_ *CallContext, _ []Value) (Value, error) { return float64(42), nil }}))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		expr     string
		bindings map[string]any
		want     any
	}{
		{`$sum()`, nil, float64(42)},
		{`($sum := function(){7}; $sum())`, nil, float64(7)},
		{`$sum`, map[string]any{"sum": nil}, nil},
		{`($x := 2; ($x := 3); $x)`, nil, float64(2)},
		{`($even := function($n){$n=0 ? true : $odd($n-1)}; $odd := function($n){$n=0 ? false : $even($n-1)}; $even(4))`, nil, true},
		{`($x := 5; $eval("$x + $sum()"))`, nil, float64(47)},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			p, err := e.CompileRaw(context.Background(), tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{Bindings: tc.bindings})
			if err != nil || got != tc.want {
				t.Fatalf("got %#v, %v; want %#v", got, err, tc.want)
			}
		})
	}
}
