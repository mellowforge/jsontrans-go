package jsontrans_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func batchDocument(t *testing.T) *jt.Document {
	t.Helper()
	d, err := jt.PrepareJSON(context.Background(), []byte(`{"a":{"n":2},"nil":null}`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestBatchSnapshotsMixedProgramsAndReusesDestination(t *testing.T) {
	ctx := context.Background()
	a, _ := jsonata.New()
	p, _ := jsonpath.New()
	pa, _ := a.Compile(ctx, "a.n")
	pp, _ := p.Compile(ctx, "$.a.n")
	missing, _ := a.Compile(ctx, "absent")
	null, _ := a.Compile(ctx, "nil")
	programs := []jt.Program{pa, pp, missing, null, identityProgram{}}
	b, err := jt.NewBatch(programs)
	if err != nil {
		t.Fatal(err)
	}
	programs[0] = nil
	dst := make([]jt.Result, 8)
	base := &dst[0]
	got, err := b.EvalPrepared(ctx, batchDocument(t), dst, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []jt.Result{{Value: float64(2), Defined: true}, {Value: []any{json.Number("2")}, Defined: true}, {}, {Defined: true}, {Value: map[string]any{"a": map[string]any{"n": json.Number("2")}, "nil": nil}, Defined: true}}
	if !reflect.DeepEqual(got, want) || &got[0] != base {
		t.Fatalf("got %#v; reused %v", got, &got[0] == base)
	}
}
func TestBatchEmptyNilAndCompilation(t *testing.T) {
	ctx := context.Background()
	d := batchDocument(t)
	b, err := jt.NewBatch(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.EvalPrepared(ctx, d, nil, jt.EvalOptions{})
	if err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
	for _, p := range []jt.Program{nil, (*jsonata.Program)(nil)} {
		if _, err := jt.NewBatch([]jt.Program{p}); err == nil {
			t.Fatal("nil program accepted")
		}
	}
	if _, err := b.EvalPrepared(nil, d, nil, jt.EvalOptions{}); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := b.EvalPrepared(ctx, nil, nil, jt.EvalOptions{}); err == nil {
		t.Fatal("nil document accepted")
	}
	var absent *jt.Batch
	if _, err := absent.EvalPrepared(ctx, d, nil, jt.EvalOptions{}); err == nil {
		t.Fatal("nil batch accepted")
	}
	a, _ := jsonata.New()
	x, _ := jt.New(map[string]jt.Processor{"ata": a})
	compiled, err := x.CompileBatch(ctx, "ata", []string{"a.n", "nil"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = compiled.EvalPrepared(ctx, d, nil, jt.EvalOptions{})
	if err != nil || len(got) != 2 || got[0].Value != float64(2) {
		t.Fatalf("%v %v", got, err)
	}
	_, err = x.CompileBatch(ctx, "ata", []string{"a.n", "("})
	var be *jt.BatchError
	var native *jt.Error
	if !errors.As(err, &be) || be.Index != 1 || !errors.As(err, &native) || native.Processor != "jsonata" {
		t.Fatalf("compile error: %v", err)
	}
}

type batchFunc func(context.Context, jt.EvalOptions) (jt.Result, error)

func (f batchFunc) Eval(ctx context.Context, _ any, o jt.EvalOptions) (jt.Result, error) {
	return f(ctx, o)
}
func (f batchFunc) EvalPrepared(ctx context.Context, _ *jt.Document, o jt.EvalOptions) (jt.Result, error) {
	return f(ctx, o)
}
func TestBatchFailFastClearsDestinationAndUnwraps(t *testing.T) {
	sentinel := errors.New("native failure")
	calls := 0
	f := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) { calls++; return jt.Result{}, sentinel })
	next := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
		t.Error("called after failure")
		return jt.Result{}, nil
	})
	b, _ := jt.NewBatch([]jt.Program{identityProgram{}, f, next})
	dst := make([]jt.Result, 3)
	got, err := b.EvalPrepared(context.Background(), batchDocument(t), dst, jt.EvalOptions{})
	var be *jt.BatchError
	if got != nil || !errors.As(err, &be) || be.Index != 1 || be.Cause != sentinel || !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if dst[0] != (jt.Result{}) {
		t.Fatalf("retained result: %#v", dst[0])
	}
}
func TestBatchBindingsAndResultsAreIndependent(t *testing.T) {
	bindings := map[string]any{"v": []any{1}}
	mutate := batchFunc(func(_ context.Context, o jt.EvalOptions) (jt.Result, error) {
		v := o.Bindings["v"].([]any)
		if v[0] != 1 {
			t.Fatal("previous expression mutated bindings")
		}
		v[0] = 2
		return jt.Result{Value: o.Bindings, Defined: true}, nil
	})
	b, _ := jt.NewBatch([]jt.Program{mutate, mutate})
	got, err := b.EvalPrepared(context.Background(), batchDocument(t), nil, jt.EvalOptions{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	got[0].Value.(map[string]any)["v"].([]any)[0] = 3
	if got[1].Value.(map[string]any)["v"].([]any)[0] != 2 || bindings["v"].([]any)[0] != 1 {
		t.Fatal("aliasing")
	}
}
func TestBatchAggregateLimitsAndCancellation(t *testing.T) {
	d := batchDocument(t)
	ctx := context.Background()
	calls := 0
	scalar := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
		calls++
		return jt.Result{Value: "abcd", Defined: true}, nil
	})
	b, _ := jt.NewBatch([]jt.Program{scalar, scalar})
	_, err := b.EvalPrepared(ctx, d, nil, jt.EvalOptions{Limits: &jt.Limits{MaxCollectionItems: 1}})
	if !errors.Is(err, jt.ErrLimit) || calls != 0 {
		t.Fatalf("count %d: %v", calls, err)
	}
	_, err = b.EvalPrepared(ctx, d, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 10}})
	var be *jt.BatchError
	if !errors.Is(err, jt.ErrLimit) || !errors.As(err, &be) || be.Index != 1 {
		t.Fatalf("aggregate: %v", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	first := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) { cancel(); return jt.Result{}, nil })
	second := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
		t.Error("called after cancellation")
		return jt.Result{}, nil
	})
	b, _ = jt.NewBatch([]jt.Program{first, second})
	_, err = b.EvalPrepared(ctx, d, nil, jt.EvalOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestBatchConcurrentReuseAndConfiguredLimits(t *testing.T) {
	ctx := context.Background()
	d := batchDocument(t)
	a, _ := jsonata.New()
	tight, _ := jsonata.New(jsonata.WithLimits(jt.Limits{MaxOutputBytes: 1}))
	p, _ := a.Compile(ctx, "a")
	q, _ := tight.Compile(ctx, "a")
	b, _ := jt.NewBatch([]jt.Program{p, p})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := b.EvalPrepared(ctx, d, nil, jt.EvalOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			got[0].Value.(map[string]any)["n"] = 10
			if got[1].Value.(map[string]any)["n"] != float64(2) {
				t.Error("cross result alias")
			}
		})
	}
	wg.Wait()
	b, _ = jt.NewBatch([]jt.Program{p, q})
	_, err := b.EvalPrepared(ctx, d, nil, jt.EvalOptions{})
	var be *jt.BatchError
	if !errors.Is(err, jt.ErrLimit) || !errors.As(err, &be) || be.Index != 1 {
		t.Fatalf("configured: %v", err)
	}
	// The outer deadline must bound adapters even when their program configuration is longer.
	wait := batchFunc(func(ctx context.Context, _ jt.EvalOptions) (jt.Result, error) {
		<-ctx.Done()
		return jt.Result{}, ctx.Err()
	})
	b, _ = jt.NewBatch([]jt.Program{wait})
	_, err = b.EvalPrepared(ctx, d, nil, jt.EvalOptions{Limits: &jt.Limits{Timeout: time.Millisecond}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

type embeddedNativeOverride struct {
	*jsonata.Program
	value any
	calls *int
}

func (p embeddedNativeOverride) EvalPrepared(context.Context, *jt.Document, jt.EvalOptions) (jt.Result, error) {
	*p.calls++
	return jt.Result{Value: p.value, Defined: true}, nil
}
func TestPreparedEmbeddingDoesNotInheritNativeTrust(t *testing.T) {
	a, _ := jsonata.New()
	p, _ := a.CompileRaw(context.Background(), "a.n")
	calls := 0
	value := map[string]any{"x": []any{1}}
	wrapper := embeddedNativeOverride{Program: p, value: value, calls: &calls}
	r, err := jt.EvalPrepared(context.Background(), wrapper, batchDocument(t), jt.EvalOptions{})
	if err != nil || calls != 1 {
		t.Fatalf("%#v %v calls=%d", r, err, calls)
	}
	r.Value.(map[string]any)["x"].([]any)[0] = 99
	if value["x"].([]any)[0] != 1 {
		t.Fatal("wrapper result not cloned")
	}
	wrapper.value = make(chan int)
	if _, err := jt.EvalPrepared(context.Background(), wrapper, batchDocument(t), jt.EvalOptions{}); !errors.Is(err, jt.ErrConversion) {
		t.Fatalf("invalid result: %v", err)
	}
}

func TestBatchConstructorsBoundCountBeforeCompilation(t *testing.T) {
	count := jt.DefaultLimits().MaxCollectionItems + 1
	if _, err := jt.NewBatch(make([]jt.Program, count)); !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("constructor count: %v", err)
	}
	processor := &batchCompileObserver{}
	x, _ := jt.New(map[string]jt.Processor{"observe": processor})
	if _, err := x.CompileBatch(context.Background(), "observe", make([]string, count)); !errors.Is(err, jt.ErrLimit) || len(processor.deadlines) != 0 {
		t.Fatalf("compile count: %v calls=%d", err, len(processor.deadlines))
	}
}

type batchCompileObserver struct{ deadlines []time.Time }

func (p *batchCompileObserver) Compile(ctx context.Context, _ string) (jt.Program, error) {
	deadline, _ := ctx.Deadline()
	p.deadlines = append(p.deadlines, deadline)
	return identityProgram{}, nil
}
func TestCompileBatchSharesOneDeadline(t *testing.T) {
	processor := &batchCompileObserver{}
	x, _ := jt.New(map[string]jt.Processor{"observe": processor})
	if _, err := x.CompileBatch(context.Background(), "observe", []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	if len(processor.deadlines) != 3 || !processor.deadlines[0].Equal(processor.deadlines[1]) || !processor.deadlines[1].Equal(processor.deadlines[2]) {
		t.Fatalf("independent compilation deadlines: %v", processor.deadlines)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := x.CompileBatch(ctx, "observe", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled empty compile: %v", err)
	}
}

func TestBatchExpressionCountersRestart(t *testing.T) {
	ctx := context.Background()
	engine, _ := jsonata.New()
	p, _ := engine.Compile(ctx, "a.n")
	d := batchDocument(t)
	options := jt.EvalOptions{Limits: &jt.Limits{MaxSteps: 30}}
	if _, err := jt.EvalPrepared(ctx, p, d, options); err != nil {
		t.Fatal(err)
	}
	programs := make([]jt.Program, 16)
	for i := range programs {
		programs[i] = p
	}
	b, err := jt.NewBatch(programs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.EvalPrepared(ctx, d, nil, options)
	if err != nil || len(got) != len(programs) {
		t.Fatalf("shared expression counters: len=%d %v", len(got), err)
	}
}

type batchLongTimeout struct{ batchFunc }

func (batchLongTimeout) Limits() jt.Limits { return jt.Limits{Timeout: time.Hour} }
func TestBatchDefaultOuterTimeoutBoundsConfiguredExpressions(t *testing.T) {
	start := time.Now()
	calls := 0
	observe := batchLongTimeout{batchFunc(func(ctx context.Context, _ jt.EvalOptions) (jt.Result, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || deadline.After(start.Add(jt.DefaultLimits().Timeout+100*time.Millisecond)) {
			t.Errorf("batch did not retain default outer deadline: %v", deadline)
		}
		return jt.Result{}, nil
	})}
	b, err := jt.NewBatch([]jt.Program{observe, observe})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.EvalPrepared(context.Background(), batchDocument(t), nil, jt.EvalOptions{}); err != nil || calls != 2 {
		t.Fatalf("%v calls=%d", err, calls)
	}
}

func TestBatchOuterInputLimitsApplyToEmptyBatch(t *testing.T) {
	b, err := jt.NewBatch(nil)
	if err != nil {
		t.Fatal(err)
	}
	d := batchDocument(t)
	for _, limits := range []jt.Limits{{MaxInputBytes: 1}, {MaxDepth: 1}, {MaxCollectionItems: 1}} {
		if _, err := b.EvalPrepared(context.Background(), d, make([]jt.Result, 0, 2), jt.EvalOptions{Limits: &limits}); !errors.Is(err, jt.ErrLimit) {
			t.Fatalf("outer dimensions %+v: %v", limits, err)
		}
	}
}

type forwardingProcessor struct{ native jt.Processor }

func (p forwardingProcessor) Compile(ctx context.Context, s string) (jt.Program, error) {
	return p.native.Compile(ctx, s)
}

type constrainedForwarder struct {
	forwardingProcessor
	limits jt.Limits
}

func (p constrainedForwarder) Limits() jt.Limits { return p.limits }
func TestPreparedForwardedNativePreservesBothConfigurations(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, input string
		limits      jt.Limits
	}{
		{"output", `{"a":"abc"}`, jt.Limits{MaxOutputBytes: 1}},
		{"input", `{"a":"abc"}`, jt.Limits{MaxInputBytes: 1}},
		{"depth", `{"a":{"b":[1]}}`, jt.Limits{MaxDepth: 2}},
		{"steps", `{"a":[0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19]}`, jt.Limits{MaxSteps: 12}},
	}
	for _, language := range []string{"jsonata", "jsonpath"} {
		for _, tc := range cases {
			for _, where := range []string{"native", "wrapper"} {
				t.Run(language+"/"+tc.name+"/"+where, func(t *testing.T) {
					var engine jt.Processor
					var err error
					nativeLimits := jt.Limits{}
					if where == "native" {
						nativeLimits = tc.limits
					}
					if language == "jsonata" {
						engine, err = jsonata.New(jsonata.WithLimits(nativeLimits))
					} else {
						engine, err = jsonpath.New(jsonpath.WithLimits(nativeLimits))
					}
					if err != nil {
						t.Fatal(err)
					}
					var adapter jt.Processor = forwardingProcessor{engine}
					if where == "wrapper" {
						adapter = constrainedForwarder{forwardingProcessor{engine}, tc.limits}
					}
					transformer, _ := jt.New(map[string]jt.Processor{"forward": adapter})
					expression := `"abc"`
					if language == "jsonpath" {
						expression = "$.a"
					}
					p, err := transformer.Compile(ctx, "forward", expression)
					if err != nil {
						t.Fatal(err)
					}
					d, err := jt.PrepareJSON(ctx, []byte(tc.input), jt.PrepareOptions{})
					if err != nil {
						t.Fatal(err)
					}
					value, _ := d.Materialize(ctx, jt.PrepareOptions{})
					if _, err = p.Eval(ctx, value, jt.EvalOptions{}); !errors.Is(err, jt.ErrLimit) {
						t.Fatalf("ordinary fixture did not exceed limits: %v", err)
					}
					if _, err = jt.EvalPrepared(ctx, p, d, jt.EvalOptions{}); !errors.Is(err, jt.ErrLimit) {
						t.Fatalf("prepared bypassed %s limits: %v", where, err)
					}
					b, err := jt.NewBatch([]jt.Program{p})
					if err != nil {
						t.Fatal(err)
					}
					if _, err = b.EvalPrepared(ctx, d, nil, jt.EvalOptions{}); !errors.Is(err, jt.ErrLimit) {
						t.Fatalf("batch bypassed %s limits: %v", where, err)
					}
					override := jt.EvalOptions{Limits: &jt.Limits{MaxInputBytes: 10000, MaxOutputBytes: 10000, MaxDepth: 100, MaxSteps: 10000}}
					if _, err = jt.EvalPrepared(ctx, p, d, override); err != nil {
						t.Fatalf("explicit override: %v", err)
					}
					if _, err = b.EvalPrepared(ctx, d, nil, override); err != nil {
						t.Fatalf("batch override: %v", err)
					}
				})
			}
		}
	}
}

func TestNativeBatchDoesNotReuseCapturedHostScopes(t *testing.T) {
	var previous *jsonata.CallContext
	engine, err := jsonata.New(jsonata.WithFunction("hold", jsonata.NativeFunction{Fn: func(call *jsonata.CallContext, _ []jsonata.Value) (jsonata.Value, error) {
		if previous != nil {
			if previous.Budget == call.Budget {
				t.Error("reused captured host budget")
			}
			if !errors.Is(previous.Context.Err(), context.Canceled) || !errors.Is(previous.Budget.Context().Err(), context.Canceled) {
				t.Error("previous host scope still active")
			}
		}
		previous = call
		return float64(1), nil
	}}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.Compile(context.Background(), "$hold()")
	if err != nil {
		t.Fatal(err)
	}
	b, err := jt.NewBatch([]jt.Program{p, p})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := b.EvalPrepared(context.Background(), batchDocument(t), nil, jt.EvalOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(previous.Context.Err(), context.Canceled) {
		t.Fatal("last host scope still active")
	}
}

func TestJSONPathBatchCancelsCapturedFunctionContexts(t *testing.T) {
	var previous context.Context
	engine, err := jsonpath.New(jsonpath.WithFunction("hold", jsonpath.Function{Parameters: []jsonpath.Type{jsonpath.ValueType}, Result: jsonpath.LogicalType, Call: func(ctx context.Context, _ []any) (any, error) {
		if previous != nil && !errors.Is(previous.Err(), context.Canceled) {
			t.Error("previous JSONPath callback scope still active")
		}
		previous = ctx
		return true, nil
	}}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.Compile(context.Background(), "$.a[?hold(@)]")
	if err != nil {
		t.Fatal(err)
	}
	b, err := jt.NewBatch([]jt.Program{p, p})
	if err != nil {
		t.Fatal(err)
	}
	d, err := jt.PrepareJSON(context.Background(), []byte(`{"a":[1]}`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.EvalPrepared(context.Background(), d, nil, jt.EvalOptions{}); err != nil {
		t.Fatal(err)
	}
	if previous == nil || !errors.Is(previous.Err(), context.Canceled) {
		t.Fatal("last JSONPath callback scope still active")
	}
}

// Failures during an expression, including a deadline or cancellation that
// occurs while it runs, identify that expression; batch-level validation
// failures are returned unwrapped.
func TestBatchErrorAttribution(t *testing.T) {
	d := batchDocument(t)
	value := batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
		return jt.Result{Value: "v", Defined: true}, nil
	})
	for name, tc := range map[string]struct{ cancelAt, want int }{
		"cancelled during the first expression": {0, 0},
		"cancelled during the last expression":  {1, 1},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			programs := []jt.Program{value, value}
			programs[tc.cancelAt] = batchFunc(func(context.Context, jt.EvalOptions) (jt.Result, error) {
				cancel()
				return jt.Result{Value: "late", Defined: true}, nil
			})
			b, _ := jt.NewBatch(programs)
			dst := make([]jt.Result, 2)
			got, err := b.EvalPrepared(ctx, d, dst, jt.EvalOptions{})
			var be *jt.BatchError
			if got != nil || !errors.Is(err, context.Canceled) || !errors.As(err, &be) || be.Index != tc.want {
				t.Fatalf("got %v, %v", got, err)
			}
			if dst[0] != (jt.Result{}) || dst[1] != (jt.Result{}) {
				t.Fatalf("destination not cleared: %#v", dst)
			}
		})
	}
	t.Run("invalid options", func(t *testing.T) {
		b, _ := jt.NewBatch([]jt.Program{value})
		_, err := b.EvalPrepared(context.Background(), d, nil, jt.EvalOptions{Limits: &jt.Limits{MaxSteps: -1}})
		var be *jt.BatchError
		if err == nil || errors.As(err, &be) {
			t.Fatalf("got %v", err)
		}
	})
}

// A wrapper's step ceiling bounds native evaluation on the ordinary path too,
// not only conversion of the input.
func TestEvalAppliesWrapperLimitsToNativeEngine(t *testing.T) {
	ctx := context.Background()
	engine, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	transformer, _ := jt.New(map[string]jt.Processor{"forward": constrainedForwarder{forwardingProcessor{engine}, jt.Limits{MaxSteps: 50}}})
	p, err := transformer.Compile(ctx, "forward", `$sum([1..500])`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Eval(ctx, nil, jt.EvalOptions{}); !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("wrapper step limit ignored: %v", err)
	}
	if r, err := p.Eval(ctx, nil, jt.EvalOptions{Limits: &jt.Limits{MaxSteps: 100000}}); err != nil || r.Value != float64(125250) {
		t.Fatalf("override: %#v, %v", r, err)
	}
}
