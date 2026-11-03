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

// preparedEntry is built only by newPreparedEntry and is read-only afterwards:
// a Batch shares its entries across concurrent EvalPrepared calls.
//
//   - configured is the effective base: wrapper ∩ native for native entries,
//     with the native RegexTimeout, otherwise the wrapper's limits.
//   - customized records whether any configuration source is non-default. It is
//     NOT configured != DefaultLimits(): a raised wrapper ceiling or a
//     wrapper-only RegexTimeout is customized even when configured is default.
//   - reuseBudget caches native.ReusableBudget() && !customized.
type preparedEntry struct {
	program     Program
	native      *prepared.Handler
	configured  Limits
	name        string
	customized  bool
	reuseBudget bool
	unsupported LimitSet
}

func newPreparedEntry(p Program) (preparedEntry, error) {
	l, err := effectiveLimits(p, nil)
	if err != nil {
		return preparedEntry{}, err
	}
	unsupported, err := unsupportedEvalLimits(p)
	if err != nil {
		return preparedEntry{}, err
	}
	entry := preparedEntry{program: p, configured: l, customized: l != DefaultLimits(), unsupported: unsupported}
	// Only the private registry wrapper may be unwrapped. External wrappers
	// retain their dispatch and must forward optional metadata explicitly.
	checked, wrapped := p.(*checkedProgram)
	if wrapped {
		entry.name = checked.name
		entry.program = checked.program
	}
	entry.native = prepared.Lookup(entry.program)
	if entry.native != nil {
		native, err := effectiveLimits(entry.program, nil)
		if err != nil {
			return preparedEntry{}, err
		}
		// Resolve overrides after intersecting both configured ceilings.
		// Regex timing belongs to the engine that executes the expression.
		entry.configured = intersectNativeLimits(l, native)
		entry.customized = entry.customized || native != DefaultLimits()
		entry.reuseBudget = entry.native.ReusableBudget() && !entry.customized
	} else if wrapped {
		// External adapters must retain the same program/wrapper intersection as Eval.
		entry.configured = checked.base
		entry.customized = checked.customized
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

// evalResolved consumes a configuration resolved for this call. l may alias the
// entry's shared configuration and must not be written through; callers may
// reuse it across entries, but budgets and bindings stay per expression.
func (e *preparedEntry) evalResolved(ctx context.Context, d *Document, o EvalOptions, l *Limits, plan prepared.BatchPlan, state any, index int, scratch *budget.Budget) (r Result, size int, err error) {
	defer func() {
		if e.name != "" {
			err = diagnostic(e.name, PhaseEvaluate, err)
		}
	}()
	if err = rejectUnsupportedLimits(e.unsupported, o.Limits); err != nil {
		return Result{}, 0, err
	}
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
		forwarded := supportedEvalLimits(l, e.unsupported)
		o.Limits = &forwarded
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
