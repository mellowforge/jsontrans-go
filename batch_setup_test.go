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
	for _, tc := range []struct {
		opts  EvalOptions
		wants [][]any
	}{
		{EvalOptions{}, [][]any{{int64(500), 40, int64(300 * time.Millisecond)}, {int64(500), 40, int64(300 * time.Millisecond)}, {int64(700), 20, int64(100 * time.Millisecond)}, {int64(700), 20, int64(100 * time.Millisecond)}, {int64(500), 40, int64(300 * time.Millisecond)}}},
		{EvalOptions{Limits: &Limits{MaxSteps: 1000, RegexTimeout: time.Second}}, [][]any{{int64(1000), 40, int64(time.Second)}, {int64(1000), 40, int64(time.Second)}, {int64(1000), 20, int64(time.Second)}, {int64(1000), 20, int64(time.Second)}, {int64(1000), 40, int64(time.Second)}}},
		{EvalOptions{Limits: &Limits{MaxSteps: 200}}, [][]any{{int64(200), 40, int64(300 * time.Millisecond)}, {int64(200), 40, int64(300 * time.Millisecond)}, {int64(200), 20, int64(100 * time.Millisecond)}, {int64(200), 20, int64(100 * time.Millisecond)}, {int64(200), 40, int64(300 * time.Millisecond)}}},
	} {
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				got, err := batch.EvalPrepared(ctx, d, nil, tc.opts)
				if err != nil {
					t.Error(err)
					return
				}
				for i, r := range got {
					if !reflect.DeepEqual(r.Value, tc.wants[i]) {
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
