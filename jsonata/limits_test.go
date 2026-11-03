package jsonata_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

// Engine limits are the defaults for every evaluation of its programs,
// including functions invoked after their evaluation ended; per-call options
// may raise them.
func TestInvokeUsesDefiningEngineLimits(t *testing.T) {
	ctx := context.Background()
	engine, err := jsonata.New(jsonata.WithLimits(jsontrans.Limits{MaxSteps: 200}))
	if err != nil {
		t.Fatal(err)
	}
	program, err := engine.CompileRaw(ctx, `function(){ $sum([1..5000]) }`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := program.EvalRaw(ctx, nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(jsonata.Callable)
	if _, err := jsonata.Invoke(ctx, fn, nil, jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("engine limit not applied: %v", err)
	}
	raised := jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 1_000_000}}
	if got, err := jsonata.Invoke(ctx, fn, nil, raised); err != nil || got != float64(12502500) {
		t.Fatalf("raised limit: got %#v, %v", got, err)
	}
}

// A per-evaluation override raises configured limits on ordinary, prepared and
// batch paths through a registry.
func TestEvaluationOverrideRaisesEngineLimits(t *testing.T) {
	ctx := context.Background()
	engine, err := jsonata.New(jsonata.WithLimits(jsontrans.Limits{MaxSteps: 200}))
	if err != nil {
		t.Fatal(err)
	}
	x, err := jsontrans.New(map[string]jsontrans.Processor{"jsonata": engine})
	if err != nil {
		t.Fatal(err)
	}
	program, err := x.Compile(ctx, "jsonata", `$sum([1..5000])`)
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsontrans.PrepareJSON(ctx, []byte(`{}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := jsontrans.NewBatch([]jsontrans.Program{program})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]func(jsontrans.EvalOptions) (any, error){
		"ordinary": func(o jsontrans.EvalOptions) (any, error) {
			r, err := program.Eval(ctx, map[string]any{}, o)
			return r.Value, err
		},
		"prepared": func(o jsontrans.EvalOptions) (any, error) {
			r, err := jsontrans.EvalPrepared(ctx, program, document, o)
			return r.Value, err
		},
		"batch": func(o jsontrans.EvalOptions) (any, error) {
			r, err := batch.EvalPrepared(ctx, document, nil, o)
			if err != nil {
				return nil, err
			}
			return r[0].Value, nil
		},
	}
	for name, eval := range paths {
		t.Run(name, func(t *testing.T) {
			if _, err := eval(jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrLimit) {
				t.Fatalf("configured limit not applied: %v", err)
			}
			got, err := eval(jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 1_000_000}})
			if err != nil || got != float64(12502500) {
				t.Fatalf("override: got %#v, %v", got, err)
			}
		})
	}
}

// Functions returned by an invoked function keep the engine's limits.
func TestNestedInvokeKeepsEngineLimits(t *testing.T) {
	ctx := context.Background()
	engine, err := jsonata.New(jsonata.WithLimits(jsontrans.Limits{MaxSteps: 200}))
	if err != nil {
		t.Fatal(err)
	}
	program, err := engine.CompileRaw(ctx, `function(){ function(){ $sum([1..5000]) } }`)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := program.EvalRaw(ctx, nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := jsonata.Invoke(ctx, outer.(jsonata.Callable), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jsonata.Invoke(ctx, inner.(jsonata.Callable), nil, jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("engine limit not applied: %v", err)
	}
}
