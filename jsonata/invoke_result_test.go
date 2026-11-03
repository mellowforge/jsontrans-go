package jsonata_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

func resultFunction(t *testing.T, expression string) jsonata.Callable {
	t.Helper()
	e, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(context.Background(), expression)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), nil, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return v.(jsonata.Callable)
}

func TestInvokeResultUsesOutputLimit(t *testing.T) {
	fn := resultFunction(t, `function(){"abcdefghij"}`)
	got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &jt.Limits{MaxInputBytes: 4, MaxOutputBytes: 12}})
	if err != nil || got != "abcdefghij" {
		t.Fatalf("output constrained by input limit: %#v, %v", got, err)
	}
}

func TestInvokeResultExactOutputSize(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		bytes int
	}{
		{`function(){"abcdefghij"}`, 12},
		{`function(){"\n"}`, 4},
		{`function(){["abcd","efgh"]}`, 15},
		{`function(){{"x":[1,2]}}`, 11},
		{`function(){"\ud800"}`, 8},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			fn := resultFunction(t, tc.expr)
			if _, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: tc.bytes}}); err != nil {
				t.Fatalf("exact size rejected: %v", err)
			}
			got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: tc.bytes - 1}})
			if got != nil || !errors.Is(err, jt.ErrLimit) {
				t.Fatalf("oversized result accepted: %#v, %v", got, err)
			}
		})
	}
}

func TestInvokePreservesRawNonfiniteResults(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		valid func(float64) bool
	}{
		{`function(){1/0}`, func(v float64) bool { return math.IsInf(v, 1) }},
		{`function(){-1/0}`, func(v float64) bool { return math.IsInf(v, -1) }},
		{`function(){0/0}`, math.IsNaN},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := jsonata.Invoke(context.Background(), resultFunction(t, tc.expr), nil, jt.EvalOptions{})
			n, ok := got.(float64)
			if err != nil || !ok || !tc.valid(n) {
				t.Fatalf("raw value lost: %#v, %v", got, err)
			}
		})
	}
	fn := resultFunction(t, `function(){[1/0,{"n":0/0}]}`)
	got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := got.([]any)
	if !math.IsInf(a[0].(float64), 1) || !math.IsNaN(a[1].(map[string]any)["n"].(float64)) {
		t.Fatalf("nested raw values lost: %#v", got)
	}
}

func TestInvokeRawResultOwnershipAndTypes(t *testing.T) {
	shared := map[string]any{"s": "text", "values": jsonata.Sequence{jsonata.Undefined, math.Inf(1)}}
	inner := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return float64(42), nil }}
	shared["fn"] = inner
	fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return shared, nil }}
	got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(map[string]any)
	seq, ok := obj["values"].(jsonata.Sequence)
	if !ok || !jsonata.IsUndefined(seq[0]) || !math.IsInf(seq[1].(float64), 1) {
		t.Fatalf("raw sequence lost: %#v", got)
	}
	obj["s"] = "changed"
	seq[0] = "changed"
	if shared["s"] != "text" || !jsonata.IsUndefined(shared["values"].(jsonata.Sequence)[0]) {
		t.Fatal("result aliases source")
	}
	if result, err := jsonata.Invoke(context.Background(), obj["fn"].(jsonata.Callable), nil, jt.EvalOptions{}); err != nil || result != float64(42) {
		t.Fatalf("returned function unusable: %v %v", result, err)
	}
}

func TestInvokeStillRejectsNonfiniteInput(t *testing.T) {
	got, err := jsonata.Invoke(context.Background(), resultFunction(t, `function($x){$x}`), []jsonata.Value{math.NaN()}, jt.EvalOptions{})
	if got != nil || !errors.Is(err, jt.ErrInvalidInput) {
		t.Fatalf("invalid input accepted: %#v, %v", got, err)
	}
}

func TestInvokeRejectsCyclicAndUnsupportedResults(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, value := range []any{cycle, make(chan int)} {
		fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return value, nil }}
		got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{})
		if got != nil || !errors.Is(err, jt.ErrInvalidInput) {
			t.Fatalf("invalid result accepted: %#v, %v", got, err)
		}
	}
}

func TestInvokeRawResultLimitsAndCancellation(t *testing.T) {
	value := []any{[]any{[]any{float64(1)}}}
	fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return value, nil }}
	for _, l := range []jt.Limits{{MaxDepth: 2}, {MaxSteps: 2}, {MaxCollectionItems: 1, MaxOutputBytes: 3}} {
		if got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &l}); got != nil || !errors.Is(err, jt.ErrLimit) {
			t.Fatalf("limit ignored: %#v, %v", got, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	canceled := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { cancel(); return value, nil }}
	if got, err := jsonata.Invoke(ctx, canceled, nil, jt.EvalOptions{}); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %#v, %v", got, err)
	}
	if !reflect.DeepEqual(value, []any{[]any{[]any{float64(1)}}}) {
		t.Fatal("conversion mutated result")
	}
}
