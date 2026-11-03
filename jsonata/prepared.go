package jsonata

import (
	"context"
	"fmt"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

// NativePrepared exposes an opaque capability sealed to this exact program.
func (p *Program) NativePrepared() *prepared.Handler { return p.prepared }

// EvalPrepared evaluates a compiled JSONata expression over a validated document.
func (p *Program) EvalPrepared(ctx context.Context, d *jsontrans.Document, opts jsontrans.EvalOptions) (jsontrans.Result, error) {
	if p == nil || p.engine == nil {
		return jsontrans.Result{}, errNotConstructed
	}
	limits, err := budget.Resolve(p.engine.limits, opts.Limits)
	if err != nil {
		return jsontrans.Result{}, err
	}
	result, err := prepared.Evaluate(ctx, d, opts.Bindings, limits, opts.Limits != nil || p.engine.limits != budget.DefaultLimits(), p.evalPrepared)
	return jsontrans.Result{Value: result.Value, Defined: result.Defined}, err
}

func (p *Program) evalPrepared(req prepared.Request) (result prepared.Result, err error) {
	defer func() { err = p.wrapTimebox(err) }()
	d, b := req.Document, req.Budget
	if d.Stats().Binary64Overflow {
		return result, fmt.Errorf("%w: number out of binary64 range", jsontrans.ErrInvalidInput)
	}
	if p.fast != nil && p.engine.evaluationDepth == 0 && len(p.engine.functions) == 0 && !req.Customized && req.Bindings == nil && d.Stats().Nodes < 10000 {
		r, ok, err := p.fast.eval(d.Root(), b)
		if err != nil {
			return result, err
		}
		if ok {
			if !r.Defined {
				return result, req.CheckExit()
			}
			result.Value, result.Bytes, err = jsonvalue.CloneDetachedSize(r.Value, b, req.Limits.MaxOutputBytes)
			if err != nil {
				return prepared.Result{}, fmt.Errorf("%w: %w", jsontrans.ErrConversion, err)
			}
			result.Defined = true
			return result, req.CheckExit()
		}
	}
	b.SetContext(jsonvalue.WithObjectOrder(b.Context()))
	input, err := document.MaterializeWithBudget(d, b)
	if err != nil {
		return result, err
	}
	input = normalize(input)
	// Materialization already validates the input; overflow is checked above.
	v, size, err := p.evaluateOwned(input, req.Bindings, b, false, true)
	if err != nil {
		return result, err
	}
	if IsUndefined(v) {
		return result, b.Check()
	}
	return prepared.Result{Value: v, Defined: true, Bytes: size}, b.Check()
}
