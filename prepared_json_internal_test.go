package jsontrans

import (
	"context"
	"errors"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

type jsonBoundaryProgram struct {
	limits Limits
	call   func(context.Context, EvalOptions) (Result, error)
}

func (p jsonBoundaryProgram) Limits() Limits { return p.limits }
func (p jsonBoundaryProgram) Eval(ctx context.Context, _ any, o EvalOptions) (Result, error) {
	return p.call(ctx, o)
}

func TestPreparedJSONRegistryLimitsAndDiagnostics(t *testing.T) {
	ctx := context.Background()
	d, err := PrepareJSON(ctx, []byte(`null`), PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, native := range []bool{false, true} {
		nativeLimits, wrapperLimits := DefaultLimits(), DefaultLimits()
		nativeLimits.MaxOutputBytes = 6
		wrapperLimits.MaxOutputBytes = 4
		var program Program = jsonBoundaryProgram{limits: nativeLimits, call: func(_ context.Context, o EvalOptions) (Result, error) {
			if o.Limits == nil || (o.Limits.MaxOutputBytes != 4 && o.Limits.MaxOutputBytes != 6) {
				t.Fatalf("forwarded options: %+v", o.Limits)
			}
			o.Limits.MaxOutputBytes = 1 // never mutate shared metadata or caller overrides
			return Result{Value: "abcd", Defined: true}, nil
		}}
		if native {
			p := &setupProgram{configured: nativeLimits}
			p.handler = prepared.New(p, func(req prepared.Request) (prepared.Result, error) {
				if req.Limits.MaxOutputBytes != 4 && req.Limits.MaxOutputBytes != 6 {
					t.Fatalf("native options: %+v", req.Limits)
				}
				return prepared.Result{Value: "abcd", Defined: true, Bytes: 6}, nil
			})
			program = p
		}
		p := mustChecked(t, "registered", program, wrapperLimits)
		b, _ := NewBatch([]Program{p})
		for _, batch := range []bool{false, true} {
			_, err := EvalPreparedJSON(ctx, p, d, EvalOptions{})
			if batch {
				_, err = b.EvalPreparedJSON(ctx, d, nil, EvalOptions{})
			}
			var diagnostic *Error
			if !errors.Is(err, ErrLimit) || !errors.As(err, &diagnostic) || diagnostic.Processor != "registered" {
				t.Fatalf("native=%t batch=%t: %v", native, batch, err)
			}
			limits := Limits{MaxOutputBytes: 6}
			o := EvalOptions{Limits: &limits}
			if batch {
				r, err := b.EvalPreparedJSON(ctx, d, nil, o)
				if err != nil || string(r[0].Value) != `"abcd"` {
					t.Fatalf("raised: %v %v", r, err)
				}
			} else {
				r, err := EvalPreparedJSON(ctx, p, d, o)
				if err != nil || string(r.Value) != `"abcd"` {
					t.Fatalf("raised: %v %v", r, err)
				}
			}
			if limits.MaxOutputBytes != 6 {
				t.Fatal("mutated caller limits")
			}
		}
	}
	bad := mustChecked(t, "registered", jsonBoundaryProgram{call: func(context.Context, EvalOptions) (Result, error) {
		return Result{Value: make(chan int), Defined: true}, nil
	}}, DefaultLimits())
	_, err = EvalPreparedJSON(ctx, bad, d, EvalOptions{})
	var diagnostic *Error
	if !errors.Is(err, ErrConversion) || !errors.As(err, &diagnostic) || diagnostic.Phase != PhaseOutput || diagnostic.Processor != "registered" {
		t.Fatalf("output diagnostic: %v", err)
	}
	// The existing Go-result API retains its original error attribution.
	_, err = EvalPrepared(ctx, bad, d, EvalOptions{})
	if !errors.As(err, &diagnostic) || diagnostic.Phase != PhaseEvaluate {
		t.Fatalf("existing API changed: %v", err)
	}
}

func TestPreparedJSONNativeDiagnosticPreserved(t *testing.T) {
	d, _ := PrepareJSON(context.Background(), []byte(`null`), PrepareOptions{})
	failure := &Error{Processor: "engine", Phase: PhaseEvaluate, Code: "native-code", Position: 7, Token: "!", Cause: context.Canceled}
	p := mustChecked(t, "registered", jsonBoundaryProgram{call: func(context.Context, EvalOptions) (Result, error) { return Result{}, failure }}, DefaultLimits())
	b, _ := NewBatch([]Program{p})
	_, err := b.EvalPreparedJSON(context.Background(), d, nil, EvalOptions{})
	var be *BatchError
	var diagnostic *Error
	if !errors.Is(err, context.Canceled) || !errors.As(err, &be) || be.Index != 0 || !errors.As(err, &diagnostic) || diagnostic.Code != "native-code" || diagnostic.Position != 7 || diagnostic.Processor != "engine" {
		t.Fatalf("diagnostics: %v", err)
	}
}
