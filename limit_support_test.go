package jsontrans

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type selectiveProgram struct {
	unsupported  LimitSet
	configured   Limits
	reads, calls atomic.Int64
	evaluate     func(context.Context, any, EvalOptions) (Result, error)
}

func (p *selectiveProgram) UnsupportedEvalLimits() LimitSet { p.reads.Add(1); return p.unsupported }
func (p *selectiveProgram) Limits() Limits                  { return p.configured }
func (p *selectiveProgram) Eval(ctx context.Context, input any, o EvalOptions) (Result, error) {
	p.calls.Add(1)
	if p.evaluate != nil {
		return p.evaluate(ctx, input, o)
	}
	return Result{Value: true, Defined: true}, nil
}

type selectivePrepared struct{ *selectiveProgram }

func (p selectivePrepared) EvalPrepared(ctx context.Context, _ *Document, o EvalOptions) (Result, error) {
	return p.Eval(ctx, nil, o)
}

func TestSelectiveLimitForwarding(t *testing.T) {
	ctx := context.Background()
	doc, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, prepared := range []bool{false, true} {
		t.Run(map[bool]string{false: "fallback", true: "prepared"}[prepared], func(t *testing.T) {
			raw := &selectiveProgram{unsupported: LimitSteps | LimitRegexTimeout, configured: Limits{Timeout: 500 * time.Millisecond, MaxDepth: 20}}
			raw.evaluate = func(_ context.Context, _ any, o EvalOptions) (Result, error) {
				if o.Limits == nil || o.Limits.MaxSteps != 0 || o.Limits.RegexTimeout != 0 {
					t.Fatalf("unsupported defaults forwarded: %+v", o.Limits)
				}
				if o.Limits.Timeout != 500*time.Millisecond && o.Limits.Timeout != time.Second {
					t.Fatalf("lost timeout: %+v", o.Limits)
				}
				if o.Limits.MaxDepth != 20 {
					t.Fatalf("program default lost: %+v", o.Limits)
				}
				o.Limits.MaxDepth = 1 // Must never alter caller options, entry or wrapper base.
				return Result{Value: true, Defined: true}, nil
			}
			var program Program = raw
			if prepared {
				program = selectivePrepared{raw}
			}
			checked := mustChecked(t, "selective", program, DefaultLimits())
			batch, err := NewBatch([]Program{checked, checked})
			if err != nil {
				t.Fatal(err)
			}
			for _, override := range []*Limits{nil, {}, {Timeout: time.Second}} {
				options := EvalOptions{Limits: override}
				before := Limits{}
				if override != nil {
					before = *override
				}
				for range 2 {
					if _, err = checked.Eval(ctx, nil, options); err != nil {
						t.Fatal(err)
					}
					if _, err = EvalJSON(ctx, checked, []byte(`null`), options); err != nil {
						t.Fatal(err)
					}
					if _, err = EvalPrepared(ctx, checked, doc, options); err != nil {
						t.Fatal(err)
					}
					if _, err = batch.EvalPrepared(ctx, doc, nil, options); err != nil {
						t.Fatal(err)
					}
				}
				if override != nil && *override != before {
					t.Fatal("caller limit pointer mutated")
				}
			}
		})
	}
}
func TestExplicitUnsupportedLimitsRejectedBeforeWork(t *testing.T) {
	ctx := context.Background()
	doc, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, override := range []Limits{{MaxSteps: 1}, {MaxSteps: DefaultLimits().MaxSteps}, {RegexTimeout: DefaultLimits().RegexTimeout}} {
		raw := &selectiveProgram{unsupported: LimitSteps | LimitRegexTimeout}
		checked := mustChecked(t, "selective", raw, DefaultLimits())
		earlier := &selectiveProgram{}
		batch, err := NewBatch([]Program{earlier, checked})
		if err != nil {
			t.Fatal(err)
		}
		cycle := map[string]any{}
		cycle["cycle"] = cycle
		options := EvalOptions{Limits: &override}
		_, err = checked.Eval(ctx, cycle, options)
		assertUnsupported(t, err, "selective")
		_, err = EvalJSON(ctx, checked, []byte(`invalid`), options)
		assertUnsupported(t, err, "")
		_, err = EvalPrepared(ctx, checked, doc, options)
		assertUnsupported(t, err, "selective")
		_, err = EvalJSON(ctx, raw, []byte(`invalid`), options)
		assertUnsupported(t, err, "")
		_, err = EvalPrepared(ctx, raw, doc, options)
		assertUnsupported(t, err, "")
		_, err = batch.EvalPrepared(ctx, doc, nil, options)
		var batchErr *BatchError
		if !errors.As(err, &batchErr) || batchErr.Index != 1 || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("batch unsupported: %v", err)
		}
		if raw.calls.Load() != 0 || earlier.calls.Load() != 0 {
			t.Fatal("callback ran for invalid options")
		}
	}
	raw := &selectiveProgram{unsupported: LimitSteps}
	p := mustChecked(t, "selective", raw, DefaultLimits())
	if _, err := p.Eval(ctx, nil, EvalOptions{Limits: &Limits{MaxSteps: -1}}); !errors.Is(err, ErrLimit) {
		t.Fatalf("negative: %v", err)
	}
}
func assertUnsupported(t *testing.T, err error, name string) {
	t.Helper()
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want unsupported: %v", err)
	}
	if name != "" {
		var diagnostic *Error
		if !errors.As(err, &diagnostic) || diagnostic.Processor != name || diagnostic.Phase != PhaseEvaluate {
			t.Fatalf("diagnostic: %v", err)
		}
	}
}

func TestUnsupportedLimitMetadataSnapshots(t *testing.T) {
	ctx := context.Background()
	raw := &selectiveProgram{unsupported: LimitSteps | LimitRegexTimeout}
	raw.evaluate = func(_ context.Context, _ any, o EvalOptions) (Result, error) {
		if o.Limits != nil && (o.Limits.MaxSteps != 0 || o.Limits.RegexTimeout != 0) {
			return Result{}, ErrUnsupported
		}
		return Result{Value: true, Defined: true}, nil
	}
	checked := mustChecked(t, "selective", raw, DefaultLimits())
	raw.unsupported = 1 << 15 // Registry metadata must already be captured.
	doc, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewBatch([]Program{checked})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			o := EvalOptions{Limits: &Limits{Timeout: time.Second}}
			if _, err := checked.Eval(ctx, nil, o); err != nil {
				t.Error(err)
			}
			if _, err := EvalPrepared(ctx, checked, doc, o); err != nil {
				t.Error(err)
			}
			if _, err := batch.EvalPrepared(ctx, doc, nil, o); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if raw.reads.Load() != 1 {
		t.Fatalf("metadata re-read %d times", raw.reads.Load())
	}
	raw = &selectiveProgram{unsupported: 1 << 15}
	if _, err := newCheckedProgram("invalid", raw, DefaultLimits()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown flags: %v", err)
	}
	if _, err := NewBatch([]Program{raw}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown batch flags: %v", err)
	}
}
func TestCommonBudgetsStillApplyWhenFieldNotForwarded(t *testing.T) {
	raw := &selectiveProgram{unsupported: LimitInputBytes | LimitOutputBytes | LimitSteps}
	configured := DefaultLimits()
	configured.MaxInputBytes, configured.MaxSteps = 4, 4
	p := mustChecked(t, "selective", raw, configured)
	if _, err := p.Eval(context.Background(), []any{1, 2, 3, 4}, EvalOptions{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("host input budget: %v", err)
	}
	if raw.calls.Load() != 0 {
		t.Fatal("oversized input reached callback")
	}
	raw.evaluate = func(context.Context, any, EvalOptions) (Result, error) {
		return Result{Value: "too large", Defined: true}, nil
	}
	configured = DefaultLimits()
	configured.MaxOutputBytes = 4
	p = mustChecked(t, "selective", raw, configured)
	if _, err := p.Eval(context.Background(), nil, EvalOptions{}); !errors.Is(err, ErrLimit) || !errors.Is(err, ErrConversion) {
		t.Fatalf("host output budget: %v", err)
	}
}

func TestEveryUnsupportedLimitField(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    LimitSet
		request Limits
		nonzero func(Limits) bool
	}{
		{"Timeout", LimitTimeout, Limits{Timeout: time.Second}, func(l Limits) bool { return l.Timeout != 0 }},
		{"MaxExpressionBytes", LimitExpressionBytes, Limits{MaxExpressionBytes: 64 << 10}, func(l Limits) bool { return l.MaxExpressionBytes != 0 }},
		{"MaxDepth", LimitDepth, Limits{MaxDepth: 256}, func(l Limits) bool { return l.MaxDepth != 0 }},
		{"MaxSteps", LimitSteps, Limits{MaxSteps: 1000000}, func(l Limits) bool { return l.MaxSteps != 0 }},
		{"MaxCollectionItems", LimitCollectionItems, Limits{MaxCollectionItems: 100000}, func(l Limits) bool { return l.MaxCollectionItems != 0 }},
		{"MaxInputBytes", LimitInputBytes, Limits{MaxInputBytes: 16 << 20}, func(l Limits) bool { return l.MaxInputBytes != 0 }},
		{"MaxOutputBytes", LimitOutputBytes, Limits{MaxOutputBytes: 16 << 20}, func(l Limits) bool { return l.MaxOutputBytes != 0 }},
		{"RegexTimeout", LimitRegexTimeout, Limits{RegexTimeout: 50 * time.Millisecond}, func(l Limits) bool { return l.RegexTimeout != 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &selectiveProgram{unsupported: tc.flag}
			raw.evaluate = func(_ context.Context, _ any, o EvalOptions) (Result, error) {
				if o.Limits == nil || tc.nonzero(*o.Limits) {
					t.Fatalf("unsupported %s default forwarded: %+v", tc.name, o.Limits)
				}
				return Result{Value: true, Defined: true}, nil
			}
			p := mustChecked(t, "selective", raw, DefaultLimits())
			if _, err := p.Eval(context.Background(), nil, EvalOptions{Limits: &Limits{}}); err != nil {
				t.Fatal(err)
			}
			before := raw.calls.Load()
			if _, err := p.Eval(context.Background(), nil, EvalOptions{Limits: &tc.request}); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("explicit %s: %v", tc.name, err)
			}
			if raw.calls.Load() != before {
				t.Fatal("unsupported override reached callback")
			}
		})
	}
}
func TestPreparedCallbackCannotRaiseHostOutputBudget(t *testing.T) {
	ctx := context.Background()
	doc, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, capable := range []bool{false, true} {
		raw := &selectiveProgram{evaluate: func(_ context.Context, _ any, o EvalOptions) (Result, error) {
			o.Limits.MaxOutputBytes = 1 << 20
			return Result{Value: "too large", Defined: true}, nil
		}}
		var program Program = raw
		if capable {
			program = selectivePrepared{raw}
		}
		configured := DefaultLimits()
		configured.MaxOutputBytes = 4
		p := mustChecked(t, "selective", program, configured)
		batch, err := NewBatch([]Program{p})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = EvalPrepared(ctx, p, doc, EvalOptions{}); !errors.Is(err, ErrLimit) || !errors.Is(err, ErrConversion) {
			t.Fatalf("prepared host budget: %v", err)
		}
		if _, err = batch.EvalPrepared(ctx, doc, nil, EvalOptions{}); !errors.Is(err, ErrLimit) || !errors.Is(err, ErrConversion) {
			t.Fatalf("batch host budget: %v", err)
		}
	}
}
func TestNestedRegistryLimitSupport(t *testing.T) {
	ctx := context.Background()
	raw := &selectiveProgram{unsupported: LimitSteps | LimitRegexTimeout}
	raw.evaluate = func(_ context.Context, _ any, o EvalOptions) (Result, error) {
		if o.Limits != nil && (o.Limits.MaxSteps != 0 || o.Limits.RegexTimeout != 0) {
			return Result{}, ErrUnsupported
		}
		return Result{Value: true, Defined: true}, nil
	}
	inner := mustChecked(t, "inner", raw, DefaultLimits())
	outer := mustChecked(t, "outer", inner, DefaultLimits())
	doc, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	options := EvalOptions{Limits: &Limits{Timeout: time.Second}}
	if _, err := outer.Eval(ctx, nil, options); err != nil {
		t.Fatal(err)
	}
	if _, err := EvalPrepared(ctx, outer, doc, options); err != nil {
		t.Fatal(err)
	}
	batch, err := NewBatch([]Program{outer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := batch.EvalPrepared(ctx, doc, nil, options); err != nil {
		t.Fatal(err)
	}
	if _, err := outer.Eval(ctx, nil, EvalOptions{Limits: &Limits{MaxSteps: 1000000}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("nested explicit: %v", err)
	}
}

func TestSupportedTimeoutStillBoundsEvaluation(t *testing.T) {
	ctx := context.Background()
	raw := &selectiveProgram{unsupported: LimitSteps | LimitRegexTimeout, evaluate: func(ctx context.Context, _ any, _ EvalOptions) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}}
	p := mustChecked(t, "selective", raw, DefaultLimits())
	doc, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewBatch([]Program{p})
	if err != nil {
		t.Fatal(err)
	}
	o := EvalOptions{Limits: &Limits{Timeout: 10 * time.Millisecond}}
	if _, err = p.Eval(ctx, nil, o); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ordinary timeout: %v", err)
	}
	if _, err = EvalPrepared(ctx, p, doc, o); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("prepared timeout: %v", err)
	}
	if _, err = batch.EvalPrepared(ctx, doc, nil, o); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("batch timeout: %v", err)
	}
}
