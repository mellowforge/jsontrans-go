package jsonata

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestPreparedSingularPaths(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	doc, err := jsontrans.PrepareJSON(ctx, []byte(`{"account":{"name":"Ada","active":true,"score":42},"unused":null}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		expr string
		want jsontrans.Result
	}{
		{"account.name", jsontrans.Result{Value: "Ada", Defined: true}},
		{"account.active", jsontrans.Result{Value: true, Defined: true}},
		{"account.score", jsontrans.Result{Value: float64(42), Defined: true}},
		{"account.missing", jsontrans.Result{}},
		{"unused", jsontrans.Result{Defined: true}},
	} {
		p, err := e.CompileRaw(ctx, tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		if p.fast == nil {
			t.Fatalf("%s: no fast plan", tc.expr)
		}
		got, err := p.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %#v, want %#v", tc.expr, got, tc.want)
		}
	}
}

func TestPreparedScalarExpressionsMatchFallback(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	input := `{"account":{"score":42,"active":true},"other":null}`
	doc, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	value, err := doc.Materialize(ctx, jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{"account.score = 42", "account.score > 10", "account.active and true", "$exists(account.score)", "$exists(missing)", "$type(account.score)", "$count(account.score)", "$length('abc')", "$sum(account.score)"} {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		if p.fast == nil {
			t.Errorf("%s: no fast plan", expr)
			continue
		}
		got, err := p.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		want, err := p.Eval(ctx, value, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, want %#v", expr, got, want)
		}
	}
}

func TestPreparedLiteralIndexAndFallback(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	input := `{"items":[{"name":"one"},{"name":"two"}],"nested":[{"name":"x"},{"name":"y"}],"n":1e400}`
	doc, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(ctx, "items[0].name")
	if err != nil {
		t.Fatal(err)
	}
	if p.fast == nil {
		t.Fatal("literal index has no fast plan")
	}
	if _, err = p.EvalPrepared(ctx, doc, jsontrans.EvalOptions{}); err == nil {
		t.Fatal("unused overflow accepted")
	}
	doc, err = jsontrans.PrepareJSON(ctx, []byte(`{"items":[{"name":"one"},{"name":"two"}],"nested":[{"name":"x"},{"name":"y"}]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
	if err != nil || !reflect.DeepEqual(got, jsontrans.Result{Value: "one", Defined: true}) {
		t.Fatalf("got %#v, %v", got, err)
	}
	p, err = e.CompileRaw(ctx, "nested.name")
	if err != nil {
		t.Fatal(err)
	}
	got, err = p.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	inputValue, err := doc.Materialize(ctx, jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := p.Eval(ctx, inputValue, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("array fallback got %#v, want %#v", got, want)
	}
}

func TestPreparedFastDifferential(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, input := range []string{`{"a":{"b":null}}`, `{"a":{"b":false}}`, `{"a":{"b":0}}`, `{"a":{"b":"\ud800"}}`, `{"a":{"b":1,"b":2}}`, `{"a":null}`} {
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, expr := range []string{`a.b`, `a.b = 2`, `a.b != null`, `$exists(a.b)`, `$type(a.b)`, `a.b and true`} {
			p, err := e.CompileRaw(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			fast, handled, err := p.fast.eval(d.Root(), mustBudget(t))
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				t.Fatalf("%s %s was not handled", input, expr)
			}
			// Match the public result conversion for isolated surrogate strings.
			if fast.Defined {
				fast.Value, err = jsonvalue.Clone(fast.Value, mustBudget(t), 1<<20)
				if err != nil {
					t.Fatal(err)
				}
			}
			want, err := p.Eval(ctx, value, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatalf("%s: %v", expr, err)
			}
			if !reflect.DeepEqual(fast, want) {
				t.Errorf("%s %s: got %#v want %#v", input, expr, fast, want)
			}
		}
	}
}

func mustBudget(t *testing.T) *budget.Budget {
	t.Helper()
	b, err := budget.New(context.Background(), budget.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func TestPreparedFunctionOverride(t *testing.T) {
	ctx := context.Background()
	e, err := New(WithFunction("exists", NativeFunction{N: 1, Fn: func(*CallContext, []Value) (Value, error) { return "overridden", nil }}))
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":1}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(ctx, `$exists(a)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	if err != nil || got.Value != "overridden" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestPreparedRootScalar(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	d, err := jsontrans.PrepareJSON(ctx, []byte(`42`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{`$`, `$$`, `$ = 42`} {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		if p.fast == nil {
			t.Fatalf("%s has no plan", expr)
		}
		got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		want, err := p.Eval(ctx, float64(42), jsontrans.EvalOptions{})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %#v versus %#v (%v)", expr, got, want, err)
		}
	}
}

func TestPreparedBindingAndLimits(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":1}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(ctx, `$exists(a)`)
	if err != nil {
		t.Fatal(err)
	}
	opts := jsontrans.EvalOptions{Bindings: map[string]any{"exists": false}}
	_, gotErr := p.EvalPrepared(ctx, d, opts)
	value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, wantErr := p.Eval(ctx, value, opts)
	var gotNative, wantNative *jsontrans.Error
	if !errors.As(gotErr, &gotNative) || !errors.As(wantErr, &wantNative) || gotNative.Code != wantNative.Code || gotNative.Position != wantNative.Position {
		t.Fatalf("binding errors %v, %v", gotErr, wantErr)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = p.EvalPrepared(cancelled, d, jsontrans.EvalOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err = p.EvalPrepared(ctx, d, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxInputBytes: 1}}); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("input limit: %v", err)
	}
}

func TestFastPlanBudgetDepth(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(ctx, strings.Repeat("true and ", 20)+"true")
	if err != nil {
		t.Fatal(err)
	}
	b, err := budget.NewFast(ctx, budget.Limits{MaxDepth: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, _, err = p.fast.eval(d.Root(), b)
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("expected depth limit, got %v", err)
	}
}

func TestFastArrayAggregates(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, input := range []string{`{"a":[1,2,3]}`, `{"a":[]}`, `{"a":[1,"bad"]}`} {
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		v, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, expr := range []string{`$count(a)`, `$sum(a)`} {
			p, err := e.CompileRaw(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			got, handled, err := p.fast.eval(d.Root(), mustBudget(t))
			if err != nil {
				t.Fatal(err)
			}
			want, werr := p.Eval(ctx, v, jsontrans.EvalOptions{})
			if werr != nil {
				if handled {
					t.Fatalf("%s %s expected fallback", input, expr)
				}
				continue
			}
			if !handled || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s %s got %#v (handled=%v), want %#v", input, expr, got, handled, want)
			}
		}
	}
}

func TestFastObjectArrayProjection(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `items.name`)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"items":[{"name":"A"},{"name":"B"}]}`, `{"items":[{}, {"name":"one"}]}`, `{"items":[]}`, `{"items":[{"name":[1,2]}]}`} {
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got, handled, err := p.fast.eval(d.Root(), mustBudget(t))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(input, `[1,2]`) {
			if handled {
				t.Fatal("nested array must fall back")
			}
			continue
		}
		want, err := p.Eval(ctx, value, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !handled || !reflect.DeepEqual(got, want) {
			t.Errorf("%s got %#v (handled=%v), want %#v", input, got, handled, want)
		}
	}
}

func TestFastFilteredProjection(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `items[score > 1].name`)
	if err != nil {
		t.Fatal(err)
	}
	if p.fast == nil {
		t.Fatal("missing fast plan")
	}
	for _, input := range []string{`{"items":[{"name":"A","score":1},{"name":"B","score":3},{"name":"C","score":4}]}`, `{"items":[{"name":"A","score":0}]}`} {
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got, handled, err := p.fast.eval(d.Root(), mustBudget(t))
		if err != nil {
			t.Fatal(err)
		}
		want, err := p.Eval(ctx, value, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !handled || !reflect.DeepEqual(got, want) {
			t.Errorf("%s got %#v (handled=%v), want %#v", input, got, handled, want)
		}
	}
}

func TestFastArrayBudgetStopsIteration(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `$sum(a)`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":[1,2,3,4,5,6,7,8,9,10]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := budget.NewFast(ctx, budget.Limits{MaxSteps: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, handled, err := p.fast.eval(d.Root(), b)
	if handled || !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
}

func TestPreparedProjectionStringsDetached(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `items.name`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"items":[{"name":"first value"},{"name":"second value"}]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	values, ok := got.Value.([]any)
	if !ok || len(values) != 2 {
		t.Fatalf("result %#v", got)
	}
	items, _ := d.Root().Member("items")
	for i, v := range values {
		item, _ := items.Index(i)
		name, _ := item.Member("name")
		source, _ := name.Text()
		result := v.(string)
		if result != source || unsafe.StringData(result) == unsafe.StringData(source) {
			t.Fatalf("string %d not detached", i)
		}
	}
}

func TestPreparedFilteredProjectionNonArray(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `items[score > 1].name`)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"items":{"score":2,"name":"X"}}`, `{"items":null}`, `{"items":42}`, `{}`} {
		t.Run(input, func(t *testing.T) {
			d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want, werr := p.Eval(ctx, value, jsontrans.EvalOptions{})
			got, gerr := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			if fmt.Sprint(gerr) != fmt.Sprint(werr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, %v; want %#v, %v", got, gerr, want, werr)
			}
		})
	}
}

func TestPreparedSumOverflowNativeError(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `$sum(a)`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":[1e308,1e308]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, wantErr := p.Eval(ctx, value, jsontrans.EvalOptions{})
	_, gotErr := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	if !errors.Is(gotErr, jsontrans.ErrConversion) || !errors.Is(wantErr, jsontrans.ErrConversion) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
		t.Fatalf("got %v; want %v", gotErr, wantErr)
	}
	_, handled, fastErr := p.fast.eval(d.Root(), mustBudget(t))
	if fastErr != nil || handled {
		t.Fatalf("overflow must fall back: handled=%v err=%v", handled, fastErr)
	}
}

func TestPreparedSumOverflowArithmeticNativeError(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `$sum(a) + 1`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":[1e308,1e308]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, wantErr := p.Eval(ctx, value, jsontrans.EvalOptions{})
	_, gotErr := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	var want, got *jsontrans.Error
	if !errors.As(wantErr, &want) || !errors.As(gotErr, &got) || want.Code != "D1001" || got.Code != want.Code || got.Position != want.Position {
		t.Fatalf("got %v; want %v", gotErr, wantErr)
	}
}

func TestPreparedDirectCopiedProgramAndNilInputs(t *testing.T) {
	ctx := context.Background()
	engine, _ := New()
	original, _ := engine.CompileRaw(ctx, "a.n")
	copied := *original
	d, _ := jsontrans.PrepareJSON(ctx, []byte(`{"a":{"n":2}}`), jsontrans.PrepareOptions{})
	for _, p := range []*Program{original, &copied} {
		got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
		if err != nil || got.Value != float64(2) {
			t.Fatalf("copy: %#v %v", got, err)
		}
		got, err = jsontrans.EvalPrepared(ctx, p, d, jsontrans.EvalOptions{})
		if err != nil || got.Value != float64(2) {
			t.Fatalf("root copy: %#v %v", got, err)
		}
		if _, err := p.EvalPrepared(ctx, nil, jsontrans.EvalOptions{}); err == nil {
			t.Fatal("nil document accepted")
		}
		if _, err := p.EvalPrepared(nil, d, jsontrans.EvalOptions{}); err == nil {
			t.Fatal("nil context accepted")
		}
	}
	var absent *Program
	if _, err := absent.EvalPrepared(ctx, d, jsontrans.EvalOptions{}); err == nil {
		t.Fatal("nil program accepted")
	}
}
