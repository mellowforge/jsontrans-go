package prepared

import (
	"context"
	"errors"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
	"testing"
)

type owner struct{ handler *Handler }

func (p *owner) NativePrepared() *Handler { return p.handler }

type embedding struct{ *owner }

func TestHandlerRequiresExactOwner(t *testing.T) {
	p := &owner{}
	p.handler = New(p, func(Request) (Result, error) { return Result{}, nil })
	if Lookup(p) != p.handler {
		t.Fatal("owner rejected")
	}
	q := *p
	for _, candidate := range []any{q, &q, embedding{p}, &embedding{p}, (*owner)(nil), struct{}{}} {
		if Lookup(candidate) != nil {
			t.Fatalf("trusted non-owner %T", candidate)
		}
	}
	if p.handler.Owner() != p {
		t.Fatal("wrong owner")
	}
}

func TestEvaluateIntoResetsCountersAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d, err := document.Parse(ctx, []byte(`null`), document.Options{})
	if err != nil {
		t.Fatal(err)
	}
	limits := budget.DefaultLimits()
	limits.MaxSteps = 2
	var scratch budget.Budget
	calls := 0
	eval := func(req Request) (Result, error) {
		calls++
		if err := req.Budget.Step(); err != nil {
			return Result{}, err
		}
		if err := req.Budget.Step(); err != nil {
			return Result{}, err
		}
		return Result{Value: true, Defined: true, Bytes: 4}, nil
	}
	for range 3 {
		if _, err := EvaluateInto(&scratch, ctx, d, nil, limits, false, eval); err != nil {
			t.Fatalf("counter did not restart: %v", err)
		}
	}
	cancel()
	if _, err := EvaluateInto(&scratch, ctx, d, nil, limits, false, eval); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("called native handler after cancellation")
	}
}
