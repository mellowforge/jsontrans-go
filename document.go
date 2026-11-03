package jsontrans

import (
	"context"
	"errors"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

// PrepareOptions controls validation work and input dimensions during preparation.
type PrepareOptions = document.Options

// Document owns validated JSON and an immutable structural index. Documents may
// be reused concurrently. Read-only string views retain the document's storage.
type Document = document.Document

// Node is an immutable document handle. Its zero value has no kind or value.
type Node = document.Node

// DocumentStats describes the validated input dimensions and numeric metadata.
type DocumentStats = document.Stats

// PreparedProgram optionally evaluates indexed documents without materializing
// the whole JSON tree. Implementations must preserve ordinary evaluation semantics.
type PreparedProgram interface {
	Program
	EvalPrepared(context.Context, *Document, EvalOptions) (Result, error)
}

// PrepareJSON copies input once, validates the complete JSON text and builds an
// immutable index. It preserves numeric spelling and isolated UTF-16 surrogates.
func PrepareJSON(ctx context.Context, input []byte, options PrepareOptions) (*Document, error) {
	return document.Parse(ctx, input, options)
}

// EvalPrepared uses an optional PreparedProgram capability, otherwise evaluates
// detached materialized input. Bindings and results are validated and detached.
// Preparation and evaluation have separate work budgets; stored input dimensions
// are checked against each evaluation's limits.
func EvalPrepared(ctx context.Context, p Program, d *Document, o EvalOptions) (Result, error) {
	if nilValue(p) || d == nil {
		return Result{}, errors.New("nil prepared program or document")
	}
	entry, err := newPreparedEntry(p)
	if err != nil {
		return Result{}, err
	}
	r, _, err := entry.eval(ctx, d, o, nil, nil, 0, nil)
	return r, err
}

type preparedEntry struct {
	program     Program
	native      *prepared.Handler
	configured  Limits
	name        string
	customized  bool
	reuseBudget bool
}

func newPreparedEntry(p Program) (preparedEntry, error) {
	l, err := effectiveLimits(p, nil)
	if err != nil {
		return preparedEntry{}, err
	}
	entry := preparedEntry{program: p, configured: l, customized: l != DefaultLimits()}
	// Only the private registry wrapper may be unwrapped. An external embedding
	// wrapper must retain its public dispatch and conversion boundary.
	if checked, ok := p.(*checkedProgram); ok {
		entry.name = checked.name
		entry.program = checked.program
	}
	entry.native = prepared.Lookup(entry.program)
	if entry.native != nil {
		native, err := effectiveLimits(entry.program, nil)
		if err != nil {
			return preparedEntry{}, err
		}
		// Resolving an override after this intersection is equivalent to
		// resolving it on both configurations first: nonzero fields replace
		// both ceilings. Regex timing always belongs to the native engine.
		entry.configured = intersectNativeLimits(l, native)
		entry.customized = entry.customized || native != DefaultLimits()
		entry.reuseBudget = entry.native.ReusableBudget() && !entry.customized
	}
	return entry, nil
}
func (e *preparedEntry) eval(ctx context.Context, d *Document, o EvalOptions, plan prepared.BatchPlan, state any, index int, scratch *budget.Budget) (Result, int, error) {
	if o.Limits == nil {
		return e.evalResolved(ctx, d, o, &e.configured, plan, state, index, scratch)
	}
	l, err := budget.Resolve(e.configured, o.Limits)
	if err != nil {
		if e.name != "" {
			err = diagnostic(e.name, PhaseEvaluate, err)
		}
		return Result{}, 0, err
	}
	return e.evalResolved(ctx, d, o, &l, plan, state, index, scratch)
}

// evalResolved consumes a configuration resolved for this call. The caller may
// reuse that immutable value, but expression budgets and bindings remain local.
func (e *preparedEntry) evalResolved(ctx context.Context, d *Document, o EvalOptions, l *Limits, plan prepared.BatchPlan, state any, index int, scratch *budget.Budget) (r Result, size int, err error) {
	defer func() {
		if e.name != "" {
			err = diagnostic(e.name, PhaseEvaluate, err)
		}
	}()
	if e.native != nil {
		return e.evalNative(ctx, d, o, l, plan, state, index, scratch)
	}
	return e.evalExternal(ctx, d, o, *l)
}
func (e *preparedEntry) evalNative(ctx context.Context, d *Document, o EvalOptions, l *Limits, plan prepared.BatchPlan, state any, index int, scratch *budget.Budget) (Result, int, error) {
	customized := e.customized || o.Limits != nil
	callback := e.native.Eval
	if plan != nil {
		callback = func(req prepared.Request) (prepared.Result, error) { return plan.Eval(index, req, state) }
	}
	var result prepared.Result
	var err error
	if scratch != nil && e.reuseBudget && o.Limits == nil && o.Bindings == nil && d.Stats().Nodes < prepared.SmallDocumentNodes {
		result, err = prepared.EvaluateInto(scratch, ctx, d, o.Bindings, *l, customized, callback)
	} else {
		result, err = prepared.Evaluate(ctx, d, o.Bindings, *l, customized, callback)
	}
	return Result{Value: result.Value, Defined: result.Defined}, result.Bytes, err
}

// Keep external option pointers and callback scopes off the native dispatch path.
func (e *preparedEntry) evalExternal(ctx context.Context, d *Document, o EvalOptions, l Limits) (r Result, size int, err error) {
	if err = d.CheckLimits(ctx, l); err != nil {
		return Result{}, 0, err
	}
	var local budget.Budget
	if err = budget.InitFastOwnedResolved(&local, ctx, l); err != nil {
		return Result{}, 0, err
	}
	defer local.Close()
	ctx = local.Context()
	if o.Bindings != nil {
		if o.Bindings, err = jsonvalue.CloneBindings(o.Bindings, &local, l.MaxInputBytes); err != nil {
			return Result{}, 0, err
		}
	}
	if o.Limits != nil || e.customized {
		o.Limits = &l
	}
	if p, ok := e.program.(PreparedProgram); ok {
		r, err = p.EvalPrepared(ctx, d, o)
	} else {
		ctx = jsonvalue.WithObjectOrder(ctx)
		var input any
		input, err = d.Materialize(ctx, PrepareOptions{Limits: &l})
		if err == nil {
			r, err = e.program.Eval(ctx, input, o)
		}
	}
	if err != nil {
		return Result{}, 0, err
	}
	if err = local.Check(); err != nil {
		return Result{}, 0, err
	}
	if !r.Defined {
		return Result{}, 0, nil
	}
	r.Value, size, err = jsonvalue.CloneDetachedSize(r.Value, &local, l.MaxOutputBytes)
	if err != nil {
		return Result{}, 0, errors.Join(ErrConversion, err)
	}
	if err = local.Check(); err != nil {
		return Result{}, 0, err
	}
	return r, size, nil
}
func (p *checkedProgram) EvalPrepared(ctx context.Context, d *Document, o EvalOptions) (Result, error) {
	return EvalPrepared(ctx, p, d, o)
}

// A registry wrapper validates under its own configuration while a forwarded
// native program still executes under its engine configuration. The stricter
// configured value applies after both are resolved with any per-evaluation
// override, so an override raises both; regex timing belongs to the engine that
// actually executes regex work.
func intersectNativeLimits(wrapper, native Limits) Limits {
	native.Timeout = min(wrapper.Timeout, native.Timeout)
	native.MaxExpressionBytes = min(wrapper.MaxExpressionBytes, native.MaxExpressionBytes)
	native.MaxDepth = min(wrapper.MaxDepth, native.MaxDepth)
	native.MaxSteps = min(wrapper.MaxSteps, native.MaxSteps)
	native.MaxCollectionItems = min(wrapper.MaxCollectionItems, native.MaxCollectionItems)
	native.MaxInputBytes = min(wrapper.MaxInputBytes, native.MaxInputBytes)
	native.MaxOutputBytes = min(wrapper.MaxOutputBytes, native.MaxOutputBytes)
	return native
}
