package jsonata

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestPreparedObjectSelectiveFields(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, expr := range []string{`{}`, `{"name":customer.name,"code":meta.id+2}`, `{"n":(meta.id*3)-1,"missing":absent,"null":nil}`, `{"\ud800":customer.name,"zero":meta.id*0}`, `{"ok":meta.id>0,"kind":$type(customer.name)}`} {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		if p.fast == nil {
			t.Fatalf("%s: missing selective object plan", expr)
		}
		for _, raw := range []string{`{"customer":{"name":"Ada"},"meta":{"id":4},"nil":null}`, `{"customer":{"name":"\ud800"},"meta":{"id":-0},"nil":false}`, `{"meta":{"id":9007199254740993}}`, `null`, `{}`} {
			d, err := jsontrans.PrepareJSON(ctx, []byte(raw), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			b, _ := budget.New(ctx, budget.Limits{})
			direct, handled, err := p.fast.eval(d.Root(), b)
			b.Close()
			// Missing numeric operands may fall back for native diagnostics/semantics.
			if err != nil {
				t.Fatal(err)
			}
			if !handled && raw != `null` && raw != `{}` && raw != `{"meta":{"id":9007199254740993}}` {
				t.Fatalf("%s %s unexpectedly fell back", expr, raw)
			}
			input, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want, werr := p.Eval(ctx, input, jsontrans.EvalOptions{})
			got, gerr := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			compareObjectResults(t, expr, raw, got, gerr, want, werr)
			if handled {
				b, _ := budget.New(ctx, budget.Limits{})
				direct.Value, err = jsonvalue.Clone(direct.Value, b, b.Limits.MaxOutputBytes)
				b.Close()
				if err != nil || !reflect.DeepEqual(direct, got) {
					t.Fatalf("direct %s: %#v/%v != %#v", expr, direct, err, got)
				}
			}
		}
	}
}

func compareObjectResults(t *testing.T, expr, raw string, got jsontrans.Result, gerr error, want jsontrans.Result, werr error) {
	t.Helper()
	if (gerr == nil) != (werr == nil) {
		t.Fatalf("%s %s: got %#v/%v want %#v/%v", expr, raw, got, gerr, want, werr)
	}
	if werr != nil {
		var g, w *jsontrans.Error
		if errors.As(werr, &w) && (!errors.As(gerr, &g) || g.Code != w.Code || g.Position != w.Position || g.Token != w.Token) {
			t.Fatalf("diagnostics: %v != %v", gerr, werr)
		}
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s %s: %#v != %#v", expr, raw, got, want)
	}
}

func TestPreparedObjectFallbackSemantics(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, expr := range []string{`{"name":customer.name,"code":meta.id+2}`, `{"a":meta.id,"a":missing}`, `{customer.name:meta.id}`, `{"a":customer}`, `{"a":meta.id*meta.id}`, `{"a":1,"b":1/0}`, `{"a":missing+meta.id}`} {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{`{"customer":[{"name":"A"},{"name":"B"}],"meta":{"id":2}}`, `{"customer":{"name":"A"},"meta":{"id":"bad"}}`, `{"meta":{"id":null}}`, `{"meta":{"id":1e308}}`, `[{"customer":{"name":"X"},"meta":{"id":1}}]`, `{}`} {
			d, err := jsontrans.PrepareJSON(ctx, []byte(raw), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			input, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got, gerr := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			want, werr := p.Eval(ctx, input, jsontrans.EvalOptions{})
			compareObjectResults(t, expr, raw, got, gerr, want, werr)
		}
	}
	p, _ := e.CompileRaw(ctx, `{"n":meta.id}`)
	d, _ := jsontrans.PrepareJSON(ctx, []byte(`{"meta":{"id":1},"unused":1e400}`), jsontrans.PrepareOptions{})
	if _, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("unused overflow: %v", err)
	}
}

func TestPreparedObjectLimitsAndOwnership(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `{"name":customer.name,"n":meta.id+1}`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"customer":{"name":"Ada"},"meta":{"id":1},"unused":[1,2,3]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []jsontrans.Limits{{MaxSteps: 1}, {MaxOutputBytes: 5}, {MaxDepth: 2}, {MaxCollectionItems: 2}, {MaxInputBytes: 5}} {
		_, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{Limits: &l})
		if !errors.Is(err, jsontrans.ErrLimit) {
			t.Fatalf("%+v: %v", l, err)
		}
	}
	// Direct fast-plan accounting remains bounded even when common wrappers
	// deliberately disable fast paths for explicit per-call limits.
	for _, l := range []budget.Limits{{MaxSteps: 1}, {MaxCollectionItems: 1}, {MaxDepth: 1}} {
		b, _ := budget.New(ctx, l)
		_, _, err := p.fast.eval(d.Root(), b)
		b.Close()
		if !errors.Is(err, budget.ErrLimit) {
			t.Fatalf("direct %+v: %v", l, err)
		}
	}
	r, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.Value.(map[string]any)["name"] = "changed"
	again, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	if err != nil || again.Value.(map[string]any)["name"] != "Ada" {
		t.Fatalf("aliased %#v %v", again, err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.EvalPrepared(cctx, d, jsontrans.EvalOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func FuzzPreparedObjectPlans(f *testing.F) {
	for _, raw := range []string{`{"customer":{"name":"Ada"},"meta":{"id":1}}`, `{"customer":{"name":"\ud800"},"meta":{"id":-0}}`, `{"customer":[{"name":"A"}],"meta":{"id":"bad"}}`, `{"meta":{"id":1e308}}`, `null`, `{}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 4096 {
			return
		}
		ctx := context.Background()
		d, err := jsontrans.PrepareJSON(ctx, raw, jsontrans.PrepareOptions{Limits: &jsontrans.Limits{MaxDepth: 32, MaxCollectionItems: 128}})
		if err != nil || d.Stats().Binary64Overflow {
			return
		}
		input, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		e, _ := New()
		for _, expr := range []string{`{"name":customer.name,"code":meta.id+2}`, `{"n":(meta.id*3)-1,"missing":absent,"null":nil}`, `{"\ud800":customer.name,"zero":meta.id*0}`, `{"ok":meta.id>0,"kind":$type(customer.name)}`} {
			p, err := e.CompileRaw(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			got, gerr := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			want, werr := p.Eval(ctx, input, jsontrans.EvalOptions{})
			compareObjectResults(t, expr, string(raw), got, gerr, want, werr)
		}
	})
}

func TestPreparedObjectBindingsAndHostOverride(t *testing.T) {
	ctx := context.Background()
	d, _ := jsontrans.PrepareJSON(ctx, []byte(`{"customer":{"name":"Ada"},"meta":{"id":1}}`), jsontrans.PrepareOptions{})
	e, _ := New(WithFunction("type", NativeFunction{N: 1, Fn: func(*CallContext, []Value) (Value, error) { return "host", nil }}))
	p, err := e.CompileRaw(ctx, `{"kind":$type(customer.name)}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
	if err != nil || got.Value.(map[string]any)["kind"] != "host" {
		t.Fatalf("host override %#v %v", got, err)
	}
	e, _ = New()
	p, _ = e.CompileRaw(ctx, `{"source":$$.meta.id,"focus":$.meta.id}`)
	opts := jsontrans.EvalOptions{Bindings: map[string]any{"$": map[string]any{"meta": map[string]any{"id": 9}}}}
	input, _ := d.Materialize(ctx, jsontrans.PrepareOptions{})
	got, gerr := p.EvalPrepared(ctx, d, opts)
	want, werr := p.Eval(ctx, input, opts)
	compareObjectResults(t, "bindings", "", got, gerr, want, werr)
}

func TestObjectUndefinedKeyOmitsValue(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, expr := range []string{`{missing:1}`, `{missing:$error("must not run"),"present":null}`, `{"a":1,missing:2,"b":3}`} {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := jsontrans.PrepareJSON(ctx, []byte(`{}`), jsontrans.PrepareOptions{})
		got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatalf("%s: undefined key should omit entry: %v", expr, err)
		}
		want := map[string]any{}
		if expr == `{missing:$error("must not run"),"present":null}` {
			want["present"] = nil
		}
		if expr == `{"a":1,missing:2,"b":3}` {
			want["a"] = float64(1)
			want["b"] = float64(3)
		}
		if !reflect.DeepEqual(got, jsontrans.Result{Value: want, Defined: true}) {
			t.Fatalf("%s: %#v != %#v", expr, got, want)
		}
	}
}
