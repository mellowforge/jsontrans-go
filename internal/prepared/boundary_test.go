package prepared

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
)

type countingContext struct {
	context.Context
	reads atomic.Int64
}

func (c *countingContext) Err() error { c.reads.Add(1); return c.Context.Err() }

func TestManagedBoundaryAvoidsRedundantContextReads(t *testing.T) {
	d, err := document.Parse(context.Background(), []byte(`null`), document.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &countingContext{Context: context.Background()}
	result, err := Evaluate(ctx, d, nil, budget.DefaultLimits(), false, func(req Request) (Result, error) {
		before := ctx.reads.Load()
		if err := req.CheckEntry(); err != nil {
			return Result{}, err
		}
		if err := req.CheckExit(); err != nil {
			return Result{}, err
		}
		if got := ctx.reads.Load() - before; got != 0 {
			t.Fatalf("enclosed handler repeated context reads: %d", got)
		}
		return Result{Value: true, Defined: true, Bytes: 4}, nil
	})
	if err != nil || !result.Defined || result.Value != true {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestManagedBoundaryStillChecksCanceledReturn(t *testing.T) {
	d, _ := document.Parse(context.Background(), []byte(`null`), document.Options{})
	for _, reuse := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		callback := func(req Request) (Result, error) {
			cancel()
			// The enclosing evaluator is responsible for the mandatory final check.
			if err := req.CheckExit(); err != nil {
				return Result{}, err
			}
			return Result{Value: true, Defined: true, Bytes: 4}, nil
		}
		var err error
		if reuse {
			var scratch budget.Budget
			_, err = EvaluateInto(&scratch, ctx, d, nil, budget.DefaultLimits(), false, callback)
		} else {
			_, err = Evaluate(ctx, d, nil, budget.DefaultLimits(), false, callback)
		}
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reuse=%t: %v", reuse, err)
		}
	}
}

func TestBareNativeBoundaryChecksCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b, err := budget.NewFast(ctx, budget.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	req := Request{Budget: b}
	cancel()
	if err := req.CheckEntry(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := req.CheckExit(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestManagedBoundaryStillChecksExpiredReturn(t *testing.T) {
	d, _ := document.Parse(context.Background(), []byte(`null`), document.Options{})
	limits := budget.DefaultLimits()
	limits.Timeout = time.Millisecond
	_, err := Evaluate(context.Background(), d, nil, limits, true, func(req Request) (Result, error) {
		<-req.Budget.Context().Done()
		if err := req.CheckExit(); err != nil {
			return Result{}, err
		}
		return Result{Value: true, Defined: true, Bytes: 4}, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
