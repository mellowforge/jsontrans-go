package jsonpath

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

func TestPreparedSingularAndFallback(t *testing.T) {
	ctx := context.Background()
	doc, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":[{"n":123456789012345678901234567890,"s":"hello"},null]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e, _ := New()
	cases := []struct {
		expr string
		want []any
	}{
		{`$.a[0].n`, []any{json.Number("123456789012345678901234567890")}},
		{`$.a[-1]`, []any{nil}},
		{`$.a[9]`, []any{}},
		{`$.a[0].s`, []any{"hello"}},
		{`$.a[*].n`, []any{json.Number("123456789012345678901234567890")}},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			p, err := e.Compile(ctx, tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			pp, ok := p.(jsontrans.PreparedProgram)
			if !ok {
				t.Fatal("JSONPath program does not support prepared evaluation")
			}
			got, err := pp.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
			if err != nil || !got.Defined || !reflect.DeepEqual(got.Value, tc.want) {
				t.Fatalf("got %#v, %v; want %#v", got, err, tc.want)
			}
		})
	}
}

func TestPreparedRejectsBindingsAndCancellation(t *testing.T) {
	ctx := context.Background()
	doc, _ := jsontrans.PrepareJSON(ctx, []byte(`{"a":1}`), jsontrans.PrepareOptions{})
	e, _ := New()
	p, _ := e.Compile(ctx, `$.a`)
	pp, ok := p.(jsontrans.PreparedProgram)
	if !ok {
		t.Fatal("missing prepared evaluator")
	}
	if _, err := pp.EvalPrepared(ctx, doc, jsontrans.EvalOptions{Bindings: map[string]any{"x": 1}}); !errors.Is(err, jsontrans.ErrUnsupported) {
		t.Fatalf("bindings: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := pp.EvalPrepared(canceled, doc, jsontrans.EvalOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestPreparedLimitsAndDetachedContainers(t *testing.T) {
	ctx := context.Background()
	doc, err := jsontrans.PrepareJSON(ctx, []byte(`{"a":{"long":"abcdefgh","n":1}}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e, _ := New()
	p, _ := e.Compile(ctx, `$.a`)
	pp := p.(jsontrans.PreparedProgram)
	r, err := pp.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.Value.([]any)[0].(map[string]any)["n"] = "changed"
	again, err := pp.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
	if err != nil || again.Value.([]any)[0].(map[string]any)["n"] != json.Number("1") {
		t.Fatalf("result changed document: %#v, %v", again, err)
	}
	for _, limits := range []*jsontrans.Limits{{MaxInputBytes: 2}, {MaxCollectionItems: 1}, {MaxOutputBytes: 2}, {MaxSteps: 1}} {
		if _, err := pp.EvalPrepared(ctx, doc, jsontrans.EvalOptions{Limits: limits}); !errors.Is(err, jsontrans.ErrLimit) {
			t.Fatalf("limits %+v: %v", limits, err)
		}
	}
	scalar, _ := e.Compile(ctx, `$.a.long`)
	if _, err := scalar.(jsontrans.PreparedProgram).EvalPrepared(ctx, doc, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxOutputBytes: 4}}); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("scalar output limit: %v", err)
	}
}

func TestPreparedConcurrentReuse(t *testing.T) {
	ctx := context.Background()
	doc, err := jsontrans.PrepareJSON(ctx, []byte(`{"items":[{"price":12345678901234567890}]}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e, _ := New()
	p, _ := e.Compile(ctx, `$.items[0].price`)
	pp := p.(jsontrans.PreparedProgram)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				got, err := pp.EvalPrepared(ctx, doc, jsontrans.EvalOptions{})
				if err != nil || !reflect.DeepEqual(got.Value, []any{json.Number("12345678901234567890")}) {
					t.Errorf("got %#v, %v", got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
