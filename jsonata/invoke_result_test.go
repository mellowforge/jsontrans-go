package jsonata_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

// constant returns a raw host function that yields v unchanged.
func constant(v jsonata.Value) jsonata.Callable {
	return jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return v, nil }}
}

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
		{`function(){{"":0}}`, 6},
		{`function(){{"":0,"a":0}}`, 12},
		{`function(){{"k":0/0}}`, 7},
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

func TestInvokeSizesNormalizedNumericResults(t *testing.T) {
	for _, tc := range []struct {
		value      any
		limit      int
		want       float64
		limitError bool
	}{
		{json.Number("1.0000"), 1, 1, false},
		{json.Number("1e-1"), 3, 0.1, false},
		{int64(999999999999999999), 18, 1e18, true},
	} {
		fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return tc.value, nil }}
		got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: tc.limit}})
		if tc.limitError {
			if got != nil || !errors.Is(err, jt.ErrLimit) {
				t.Errorf("oversized normalized number: %#v, %v", got, err)
			}
		} else if err != nil || got != tc.want {
			t.Errorf("normalized output: %#v, %v, want %v", got, err, tc.want)
		}
	}
}

func TestInvokeRejectsCanonicalKeyCollisions(t *testing.T) {
	value := map[string]any{"😀": float64(1), "\xed\xa0\xbd\xed\xb8\x80": float64(2)}
	fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return value, nil }}
	got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{})
	if got != nil || !errors.Is(err, jt.ErrInvalidInput) {
		t.Fatalf("canonical keys overwritten: %#v, %v", got, err)
	}
}

func TestInvokePropagatesSequenceConversionLimit(t *testing.T) {
	cycle := make(jsonata.Sequence, 1)
	cycle[0] = cycle
	fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return cycle, nil }}
	got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &jt.Limits{MaxDepth: 4}})
	if got != nil || !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("conversion failure lost: %#v, %v", got, err)
	}
}

func TestInvokeNormalizedNumericSizeIsCumulative(t *testing.T) {
	for _, value := range []any{[]any{json.Number("1e3")}, []any{float32(1.2)}} {
		fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return value, nil }}
		if got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 5}}); got != nil || !errors.Is(err, jt.ErrLimit) {
			t.Fatalf("nested normalized size ignored: %#v, %v", got, err)
		}
	}
	for _, value := range []json.Number{"bad", "NaN", "1e9999"} {
		fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return value, nil }}
		if got, err := jsonata.Invoke(context.Background(), fn, nil, jt.EvalOptions{}); got != nil || !errors.Is(err, jt.ErrInvalidInput) {
			t.Fatalf("invalid Go number admitted: %#v, %v", got, err)
		}
	}
}

func FuzzInvokeResultJSON(f *testing.F) {
	for _, s := range []string{`null`, `1`, `"\n"`, `{"x":[1,2]}`, `["abc",false,{}]`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 2048 {
			return
		}
		var value any
		if err := json.Unmarshal([]byte(s), &value); err != nil {
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return
		}
		fn := jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) { return value, nil }}
		opts := jt.EvalOptions{Limits: &jt.Limits{MaxInputBytes: 1, MaxOutputBytes: len(encoded), MaxDepth: 4096}}
		got, err := jsonata.Invoke(context.Background(), fn, nil, opts)
		if err != nil || !reflect.DeepEqual(got, value) {
			t.Fatalf("raw result mismatch: %#v, %v; want %#v", got, err, value)
		}
		if len(encoded) > 1 {
			opts.Limits.MaxOutputBytes--
			if got, err := jsonata.Invoke(context.Background(), fn, nil, opts); got != nil || !errors.Is(err, jt.ErrLimit) {
				t.Fatalf("size limit ignored: %#v, %v", got, err)
			}
		}
	})
}

// A container reused by sibling values is not a cycle.
func TestInvokeRawResultSharedSiblings(t *testing.T) {
	s := []any{float64(1)}
	m := map[string]any{"a": float64(1)}
	for _, value := range []any{[]any{s, s}, []any{m, m}, map[string]any{"x": s, "y": s}, map[string]any{"x": m, "y": m}} {
		got, err := jsonata.Invoke(context.Background(), constant(value), nil, jt.EvalOptions{})
		if err != nil || !reflect.DeepEqual(got, value) {
			t.Fatalf("shared sibling rejected: %#v, %v", got, err)
		}
	}
	got, err := jsonata.Invoke(context.Background(), resultFunction(t, `($a := [[1]]; function(){[$a, $a]})`), nil, jt.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got, []any{[]any{float64(1)}, []any{float64(1)}}) {
		t.Fatalf("language sibling rejected: %#v, %v", got, err)
	}
}

func TestInvokeRejectsArrayCycleResult(t *testing.T) {
	cycle := make([]any, 1)
	cycle[0] = cycle
	got, err := jsonata.Invoke(context.Background(), constant(cycle), nil, jt.EvalOptions{})
	if got != nil || !errors.Is(err, jt.ErrInvalidInput) {
		t.Fatalf("array cycle accepted: %#v, %v", got, err)
	}
}

func TestInvokeRawResultCollectionLimit(t *testing.T) {
	for _, value := range []any{
		[]any{float64(1), float64(2), float64(3)},
		map[string]any{"a": float64(1), "b": float64(2), "c": float64(3)},
	} {
		if _, err := jsonata.Invoke(context.Background(), constant(value), nil, jt.EvalOptions{Limits: &jt.Limits{MaxCollectionItems: 3}}); err != nil {
			t.Fatalf("collection at the limit rejected: %v", err)
		}
		got, err := jsonata.Invoke(context.Background(), constant(value), nil, jt.EvalOptions{Limits: &jt.Limits{MaxCollectionItems: 2}})
		if got != nil || !errors.Is(err, jt.ErrLimit) {
			t.Fatalf("collection limit ignored: %#v, %v", got, err)
		}
	}
}

// Public UTF-16 types become ordinary language strings and objects.
func TestInvokeRawResultUTF16Types(t *testing.T) {
	lone := "\xed\xa0\x80"
	for _, tc := range []struct{ value, want any }{
		{jt.UTF16String(lone), lone},
		{[]any{jt.UTF16String(lone)}, []any{lone}},
		{jt.UTF16Object{lone: jt.UTF16String(lone)}, map[string]any{lone: lone}},
	} {
		got, err := jsonata.Invoke(context.Background(), constant(tc.value), nil, jt.EvalOptions{})
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("UTF-16 value %#v: %#v, %v", tc.value, got, err)
		}
	}
	got, err := jsonata.Invoke(context.Background(), constant(jt.UTF16Object{lone: math.Inf(1)}), nil, jt.EvalOptions{})
	m, ok := got.(map[string]any)
	if err != nil || !ok || !math.IsInf(m[lone].(float64), 1) {
		t.Fatalf("UTF-16 object lost raw value: %#v, %v", got, err)
	}
}

func TestInvokeRawResultNilContainersAreNull(t *testing.T) {
	for _, value := range []any{[]any(nil), map[string]any(nil)} {
		got, err := jsonata.Invoke(context.Background(), constant(value), nil, jt.EvalOptions{})
		if err != nil || got != nil {
			t.Fatalf("nil container %#v: %#v, %v", value, got, err)
		}
	}
}

// Language-only scalars occupy a nesting level like any JSON value.
func TestInvokeRawLanguageScalarDepth(t *testing.T) {
	for depth := 1; depth <= 6; depth++ {
		l := jt.Limits{MaxDepth: depth}
		_, jsonErr := jsonata.Invoke(context.Background(), constant([]any{[]any{float64(1)}}), nil, jt.EvalOptions{Limits: &l})
		_, rawErr := jsonata.Invoke(context.Background(), constant([]any{[]any{math.NaN()}}), nil, jt.EvalOptions{Limits: &l})
		if (jsonErr == nil) != (rawErr == nil) {
			t.Fatalf("depth %d: JSON %v, raw %v", depth, jsonErr, rawErr)
		}
	}
}

// Long json.Number spellings consume evaluation steps while being checked.
func TestInvokeLongNumberResultChargesSteps(t *testing.T) {
	short, long := json.Number("1.0"), json.Number("1."+strings.Repeat("0", 40000))
	steps := int64(1)
	for ; steps < 100; steps++ {
		if _, err := jsonata.Invoke(context.Background(), constant(short), nil, jt.EvalOptions{Limits: &jt.Limits{MaxSteps: steps}}); err == nil {
			break
		}
	}
	got, err := jsonata.Invoke(context.Background(), constant(long), nil, jt.EvalOptions{Limits: &jt.Limits{MaxSteps: steps}})
	if got != nil || !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("long number charged no steps at MaxSteps %d: %#v, %v", steps, got, err)
	}
}
