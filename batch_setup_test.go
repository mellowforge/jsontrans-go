package jsontrans

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

type setupProgram struct {
	configured Limits
	handler    *prepared.Handler
}

func (p *setupProgram) Limits() Limits                    { return p.configured }
func (p *setupProgram) NativePrepared() *prepared.Handler { return p.handler }
func (p *setupProgram) Eval(context.Context, any, EvalOptions) (Result, error) {
	panic("expected native dispatch")
}

// Mixed wrapper/native defaults must retain both ceilings, the native regex
// timeout and the same explicit override across adjacent and alternating entries.
func TestBatchAlternatingResolvedConfigurations(t *testing.T) {
	ctx := context.Background()
	d, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	makeProgram := func(native, wrapper Limits) Program {
		p := &setupProgram{configured: native}
		p.handler = prepared.New(p, func(req prepared.Request) (prepared.Result, error) {
			l := req.Limits
			return prepared.Result{Value: []any{int64(l.MaxSteps), l.MaxDepth, int64(l.RegexTimeout)}, Defined: true, Bytes: 32}, nil
		})
		return &checkedProgram{name: "test", program: p, configured: wrapper}
	}
	a, b := DefaultLimits(), DefaultLimits()
	a.MaxSteps, a.MaxDepth, a.RegexTimeout = 500, 40, 300*time.Millisecond
	b.MaxSteps, b.MaxDepth, b.RegexTimeout = 900, 20, 100*time.Millisecond
	wrapper := DefaultLimits()
	wrapper.MaxSteps = 700
	p, q := makeProgram(a, wrapper), makeProgram(b, wrapper)
	batch, err := NewBatch([]Program{p, p, q, q, p})
	if err != nil {
		t.Fatal(err)
	}
	want := func(steps int64, depth int, regex time.Duration) []any { return []any{steps, depth, int64(regex)} }
	// Entries run in the order p, p, q, q, p.
	pattern := func(p, q []any) [][]any { return [][]any{p, p, q, q, p} }
	for _, tc := range []struct {
		opts  EvalOptions
		wants [][]any
	}{
		{EvalOptions{}, pattern(want(500, 40, 300*time.Millisecond), want(700, 20, 100*time.Millisecond))},
		{EvalOptions{Limits: &Limits{MaxSteps: 1000, RegexTimeout: time.Second}}, pattern(want(1000, 40, time.Second), want(1000, 20, time.Second))},
		{EvalOptions{Limits: &Limits{MaxSteps: 200}}, pattern(want(200, 40, 300*time.Millisecond), want(200, 20, 100*time.Millisecond))},
	} {
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				got, err := batch.EvalPrepared(ctx, d, nil, tc.opts)
				if err != nil {
					t.Error(err)
					return
				}
				if len(got) != len(tc.wants) {
					t.Errorf("results: %d, want %d", len(got), len(tc.wants))
					return
				}
				for i, r := range got {
					if !r.Defined || !reflect.DeepEqual(r.Value, tc.wants[i]) {
						t.Errorf("entry %d: %#v, want %#v", i, r.Value, tc.wants[i])
					}
				}
			})
		}
		wg.Wait()
	}
}

type mutatingSetupProgram struct{ seen []int64 }

func (p *mutatingSetupProgram) Limits() Limits { l := DefaultLimits(); l.MaxSteps = 300; return l }
func (p *mutatingSetupProgram) Eval(_ context.Context, _ any, o EvalOptions) (Result, error) {
	p.seen = append(p.seen, o.Limits.MaxSteps)
	o.Limits.MaxSteps = 1
	return Result{Value: true, Defined: true}, nil
}

// An external adapter receives its own option copy for each expression. Mutating
// that copy must not affect a cached configuration, the caller, or later calls.
func TestBatchResolvedOptionsStayPrivate(t *testing.T) {
	ctx := context.Background()
	d, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := &mutatingSetupProgram{}
	b, err := NewBatch([]Program{p, p})
	if err != nil {
		t.Fatal(err)
	}
	l := Limits{MaxSteps: 400}
	for range 2 {
		if _, err := b.EvalPrepared(ctx, d, nil, EvalOptions{Limits: &l}); err != nil {
			t.Fatal(err)
		}
	}
	if l.MaxSteps != 400 || !reflect.DeepEqual(p.seen, []int64{400, 400, 400, 400}) {
		t.Fatalf("limits escaped: %#v, seen %v", l, p.seen)
	}
}

type limitsProcessor struct{ seen *[]Limits }

func (p limitsProcessor) Limits() Limits { l := DefaultLimits(); l.MaxSteps = 77; return l }
func (p limitsProcessor) Compile(context.Context, string) (Program, error) {
	return limitsRecorder(p), nil
}

// limitsRecorder deliberately does not implement LimitsProvider: only its
// processor reports configured limits.
type limitsRecorder limitsProcessor

func (p limitsRecorder) Eval(_ context.Context, _ any, o EvalOptions) (Result, error) {
	if o.Limits == nil {
		*p.seen = append(*p.seen, Limits{})
	} else {
		*p.seen = append(*p.seen, *o.Limits)
	}
	return Result{Value: true, Defined: true}, nil
}

// Registry-configured limits reach external adapters on prepared paths, with
// or without a per-call override.
func TestPreparedExternalReceivesConfiguredLimits(t *testing.T) {
	ctx := context.Background()
	var seen []Limits
	x, err := New(map[string]Processor{"x": limitsProcessor{seen: &seen}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := x.Compile(ctx, "x", "e")
	if err != nil {
		t.Fatal(err)
	}
	b, err := x.CompileBatch(ctx, "x", []string{"e"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []EvalOptions{{}, {Limits: &Limits{MaxDepth: 5}}} {
		if _, err := EvalPrepared(ctx, p, d, o); err != nil {
			t.Fatal(err)
		}
		if _, err := b.EvalPrepared(ctx, d, nil, o); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("calls: %d", len(seen))
	}
	for i, l := range seen {
		if l.MaxSteps != 77 || (i >= 2) != (l.MaxDepth == 5) {
			t.Fatalf("call %d received %+v", i, l)
		}
	}
}

// A per-call override marks native requests as customized, which disables
// engine fast paths; default calls stay uncustomized on both paths.
func TestPreparedOverrideMarksNativeRequestCustomized(t *testing.T) {
	ctx := context.Background()
	var customized []bool
	p := &setupProgram{configured: DefaultLimits()}
	p.handler = prepared.NewReusable(p, func(req prepared.Request) (prepared.Result, error) {
		customized = append(customized, req.Customized)
		return prepared.Result{Value: true, Defined: true, Bytes: 4}, nil
	})
	b, err := NewBatch([]Program{p})
	if err != nil {
		t.Fatal(err)
	}
	d, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []EvalOptions{{}, {Limits: &Limits{MaxSteps: 500}}} {
		if _, err := EvalPrepared(ctx, p, d, o); err != nil {
			t.Fatal(err)
		}
		if _, err := b.EvalPrepared(ctx, d, nil, o); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(customized, []bool{false, false, true, true}) {
		t.Fatalf("customized flags: %v", customized)
	}
}

// A wrapper that raises a ceiling or sets only a regex timeout customizes the
// entry even though the intersected native limits stay at the defaults.
func TestPreparedWrapperCustomizationWithDefaultEffectiveLimits(t *testing.T) {
	ctx := context.Background()
	d, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raised, regex := DefaultLimits(), DefaultLimits()
	raised.MaxSteps *= 2
	regex.RegexTimeout *= 2
	for _, wrapper := range []Limits{raised, regex} {
		var customized []bool
		native := &setupProgram{configured: DefaultLimits()}
		native.handler = prepared.NewReusable(native, func(req prepared.Request) (prepared.Result, error) {
			if req.Limits != DefaultLimits() {
				t.Errorf("effective limits %+v", req.Limits)
			}
			customized = append(customized, req.Customized)
			return prepared.Result{Value: true, Defined: true, Bytes: 4}, nil
		})
		p := &checkedProgram{name: "test", program: native, configured: wrapper}
		b, err := NewBatch([]Program{p})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := EvalPrepared(ctx, p, d, EvalOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := b.EvalPrepared(ctx, d, nil, EvalOptions{}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(customized, []bool{true, true}) {
			t.Fatalf("wrapper %+v: customized %v", wrapper, customized)
		}
	}
}
