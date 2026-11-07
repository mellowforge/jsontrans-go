package jsontrans_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func TestPreparedJSONParity(t *testing.T) {
	ctx := context.Background()
	a, _ := jsonata.New()
	p, _ := jsonpath.New()
	input := []byte(`{"n":9007199254740993,"nil":null,"a":[[1],[],2],"s":"\ud800","\udfff":"<>&"}`)
	d, err := jt.PrepareJSON(ctx, input, jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var programs []jt.Program
	for _, tc := range []struct {
		engine      jt.Processor
		expressions []string
	}{
		{a, []string{"n", "nil", "absent", "a", "s", "$"}},
		{p, []string{"$.n", "$.nil", "$.absent", "$.a", "$.s", "$", "$['n','n']"}},
	} {
		for _, expression := range tc.expressions {
			program, err := tc.engine.Compile(ctx, expression)
			if err != nil {
				t.Fatal(err)
			}
			programs = append(programs, program)
		}
	}
	b, err := jt.NewBatch(programs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, program := range programs {
		want, err := jt.EvalJSON(ctx, program, input, jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		single, err := jt.EvalPreparedJSON(ctx, program, d, jt.EvalOptions{})
		if err != nil || !reflect.DeepEqual(single, want) || !reflect.DeepEqual(got[i], want) {
			t.Fatalf("expression %d: single=%s batch=%s want=%s error=%v", i, single.Value, got[i].Value, want.Value, err)
		}
	}
	if got[2].Defined || got[2].Value != nil || !got[1].Defined || string(got[1].Value) != "null" || string(got[8].Value) != "[]" {
		t.Fatalf("missing/null/list shapes: %#v", got)
	}
}

func TestPreparedJSONOwnershipAndConcurrency(t *testing.T) {
	ctx := context.Background()
	d := batchDocument(t)
	b, _ := jt.NewBatch([]jt.Program{identityProgram{}, identityProgram{}})
	dst := make([]jt.JSONResult, 2, 4)
	first, err := b.EvalPreparedJSON(ctx, d, dst, jt.EvalOptions{})
	if err != nil || &first[0] != &dst[0] {
		t.Fatalf("reuse: %v", err)
	}
	retained := first[0].Value
	want := string(retained)
	second, err := b.EvalPreparedJSON(ctx, d, dst, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second[0].Value[0] = 'x'
	if string(retained) != want || string(second[1].Value) != want {
		t.Fatal("aliased output storage")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				r, err := b.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{})
				if err != nil || len(r) != 2 || string(r[0].Value) != want {
					t.Errorf("concurrent: %v %v", r, err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestPreparedJSONActualEncodedLimitsAndFailure(t *testing.T) {
	ctx := context.Background()
	d := batchDocument(t)
	value := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
		return jt.Result{Value: "<", Defined: true}, nil
	})
	b, _ := jt.NewBatch([]jt.Program{value, value})
	dst := make([]jt.JSONResult, 2)
	// Each HTML-escaped JSON string is eight bytes, rather than three.
	r, err := b.EvalPreparedJSON(ctx, d, dst, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 15}})
	var be *jt.BatchError
	if r != nil || !errors.Is(err, jt.ErrLimit) || !errors.As(err, &be) || be.Index != 1 || dst[0].Defined || dst[0].Value != nil {
		t.Fatalf("aggregate: %v %v dst=%v", r, err, dst)
	}
	r, err = b.EvalPreparedJSON(ctx, d, dst, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 16}})
	if err != nil || len(r) != 2 {
		t.Fatalf("exact aggregate: %v %v", r, err)
	}
	_, err = jt.EvalPreparedJSON(ctx, value, d, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 7}})
	if !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("single output size: %v", err)
	}
	bad := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
		return jt.Result{Value: func() {}, Defined: true}, nil
	})
	b, _ = jt.NewBatch([]jt.Program{value, bad, value})
	r, err = b.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{})
	var diagnostic *jt.Error
	if r != nil || !errors.Is(err, jt.ErrConversion) || !errors.As(err, &be) || be.Index != 1 || !errors.As(err, &diagnostic) || diagnostic.Phase != jt.PhaseOutput {
		t.Fatalf("conversion: %v %v", r, err)
	}
}

func TestPreparedJSONMissingAndNullAccounting(t *testing.T) {
	ctx := context.Background()
	a, _ := jsonata.New()
	missing, _ := a.Compile(ctx, "absent")
	null, _ := a.Compile(ctx, "nil")
	b, _ := jt.NewBatch([]jt.Program{missing, null, missing})
	r, err := b.EvalPreparedJSON(ctx, batchDocument(t), nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 4}})
	if err != nil || len(r) != 3 || r[0].Defined || !r[1].Defined || string(r[1].Value) != "null" || r[2].Defined {
		t.Fatalf("missing accounting: %+v %v", r, err)
	}
}

func TestPreparedJSONRejectsInvalidExternalResults(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, v := range []any{cycle, math.Inf(1), json.Number("invalid"), []any{make(chan int)}} {
		p := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
			return jt.Result{Value: v, Defined: true}, nil
		})
		r, err := jt.EvalPreparedJSON(context.Background(), p, batchDocument(t), jt.EvalOptions{})
		var e *jt.Error
		if r.Defined || r.Value != nil || !errors.Is(err, jt.ErrConversion) || !errors.As(err, &e) || e.Phase != jt.PhaseOutput {
			t.Fatalf("invalid output: %T result=%+v error=%v", v, r, err)
		}
	}
}

func TestPreparedJSONPreflightAndEmpty(t *testing.T) {
	ctx := context.Background()
	d := batchDocument(t)
	b, _ := jt.NewBatch([]jt.Program{identityProgram{}})
	dst := []jt.JSONResult{{Value: json.RawMessage(`"sentinel"`), Defined: true}}
	for _, limits := range []jt.Limits{{MaxSteps: -1}, {MaxInputBytes: 1}} {
		if _, err := b.EvalPreparedJSON(ctx, d, dst, jt.EvalOptions{Limits: &limits}); err == nil {
			t.Fatal("accepted invalid options/input")
		}
		if string(dst[0].Value) != `"sentinel"` {
			t.Fatal("preflight changed destination")
		}
	}
	empty, _ := jt.NewBatch(nil)
	if _, err := empty.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{Limits: &jt.Limits{MaxDepth: 1}}); !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("empty input bounds: %v", err)
	}
	if r, err := empty.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{}); err != nil || len(r) != 0 {
		t.Fatalf("empty: %v %v", r, err)
	}
	var absent *jt.Batch
	if _, err := absent.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{}); err == nil {
		t.Fatal("nil batch")
	}
	for _, tc := range []struct {
		ctx context.Context
		p   jt.Program
		d   *jt.Document
	}{{nil, identityProgram{}, d}, {ctx, nil, d}, {ctx, identityProgram{}, nil}} {
		if _, err := jt.EvalPreparedJSON(tc.ctx, tc.p, tc.d, jt.EvalOptions{}); err == nil {
			t.Fatal("nil argument")
		}
	}
}

func TestPreparedJSONCallbackScopeAndCancellation(t *testing.T) {
	d := batchDocument(t)
	var captured context.Context
	p := batchFunc(func(ctx context.Context, o jt.EvalOptions) (jt.Result, error) {
		captured = ctx
		o.Bindings["v"].([]any)[0] = 99
		return jt.Result{Value: o.Bindings, Defined: true}, nil
	})
	o := jt.EvalOptions{Bindings: map[string]any{"v": []any{1}}}
	r, err := jt.EvalPreparedJSON(context.Background(), p, d, o)
	if err != nil || string(r.Value) != `{"v":[99]}` || !errors.Is(captured.Err(), context.Canceled) || o.Bindings["v"].([]any)[0] != 1 {
		t.Fatalf("scope/ownership: %s %v", r.Value, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := jt.EvalPreparedJSON(ctx, p, d, o); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	wait := batchFunc(func(ctx context.Context, _ jt.EvalOptions) (jt.Result, error) {
		<-ctx.Done()
		return jt.Result{}, ctx.Err()
	})
	b, _ := jt.NewBatch([]jt.Program{p, wait})
	_, err = b.EvalPreparedJSON(context.Background(), d, nil, jt.EvalOptions{Bindings: o.Bindings, Limits: &jt.Limits{Timeout: time.Millisecond}})
	var be *jt.BatchError
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &be) || be.Index != 1 {
		t.Fatalf("deadline: %v", err)
	}
}
