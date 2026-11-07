package jsontrans

import (
	"context"
	"errors"
	"fmt"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

// EvalPreparedJSON evaluates a compiled program against an immutable document
// and encodes its result as JSON. It supports every Program, using detached
// materialized input when prepared evaluation is unavailable. The returned
// bytes are owned by the result; missing has Defined false and nil Value.
// Evaluation and encoding share a deadline and configured dimension limits.
// Preparation has its own budget. Bindings are validated and detached.
func EvalPreparedJSON(ctx context.Context, p Program, d *Document, o EvalOptions) (JSONResult, error) {
	if ctx == nil || nilValue(p) || d == nil {
		return JSONResult{}, errors.New("nil context, prepared program, or document")
	}
	entry, err := newPreparedEntry(p)
	if err != nil {
		return JSONResult{}, err
	}
	l, err := budget.Resolve(entry.configured, o.Limits)
	if err != nil {
		return JSONResult{}, diagnostic(entry.name, PhaseEvaluate, err)
	}
	r, _, err := entry.evalJSON(ctx, d, o, l, nil, nil, 0, nil)
	return r, err
}

// EvalPreparedJSON evaluates and encodes each expression in order. It reuses dst
// slice capacity, but never overwrites previously returned byte buffers. Each
// result owns its JSON bytes. Concurrent calls require separate destinations.
// A batch has an overall deadline, count/input limits and a limit on the sum of
// encoded output bytes, in addition to each program's limits. Evaluation and
// encoding share each expression's deadline. On failure it returns nil and
// clears populated dst slots; other contents are unspecified. Preflight failures
// leave dst untouched. BatchError identifies the first failing expression.
func (b *Batch) EvalPreparedJSON(ctx context.Context, d *Document, dst []JSONResult, o EvalOptions) ([]JSONResult, error) {
	var outer budget.Budget
	defer outer.Close()
	limits, err := b.initPreparedScope(&outer, ctx, d, o)
	if err != nil {
		return nil, err
	}
	count := len(b.entries)
	if cap(dst) < count {
		dst = make([]JSONResult, count)
	} else {
		dst = dst[:count]
	}
	var state any
	if count > 0 && b.plan != nil {
		state = b.plan.NewState(d)
	}
	bytes := 0
	var nativeScratch budget.Budget
	var previousBase, resolved Limits
	haveResolved := false
	for i := range b.entries {
		entry := &b.entries[i]
		fail := func(err error) ([]JSONResult, error) {
			clear(dst[:i])
			return nil, &BatchError{Index: i, Cause: err}
		}
		if err := outer.Check(); err != nil {
			return fail(err)
		}
		l := &entry.configured
		if o.Limits != nil {
			if !haveResolved || previousBase != *l {
				resolved, err = budget.Resolve(*l, o.Limits)
				if err != nil {
					return fail(diagnostic(entry.name, PhaseEvaluate, err))
				}
				previousBase, haveResolved = *l, true
			}
			l = &resolved
		}
		r, size, err := entry.evalJSON(outer.Context(), d, o, *l, b.plan, state, i, &nativeScratch)
		if err != nil {
			return fail(err)
		}
		if size > limits.MaxOutputBytes-bytes {
			return fail(preparedJSONError(entry.name, fmt.Errorf("%w: aggregate batch output size", ErrLimit)))
		}
		bytes += size
		dst[i] = r
	}
	return dst, nil
}

func (e preparedEntry) evalJSON(ctx context.Context, d *Document, o EvalOptions, l Limits, plan prepared.BatchPlan, state any, index int, scratch *budget.Budget) (JSONResult, int, error) {
	// Start before evaluation, so encoding cannot acquire a fresh timeout when
	// the evaluator closes its child scope. Never mutate a shared batch entry.
	var scope budget.Budget
	if err := budget.InitFastResolved(&scope, ctx, l); err != nil {
		return JSONResult{}, 0, diagnostic(e.name, PhaseEvaluate, err)
	}
	defer scope.Close()
	e.jsonOutput = true
	r, _, err := e.evalResolved(scope.Context(), d, o, &l, plan, state, index, scratch)
	if err != nil {
		return JSONResult{}, 0, err
	}
	if err := scope.Check(); err != nil {
		return JSONResult{}, 0, preparedJSONError(e.name, err)
	}
	if !r.Defined {
		return JSONResult{}, 0, nil
	}
	data, err := jsonvalue.Encode(r.Value, &scope)
	if err != nil {
		return JSONResult{}, 0, preparedJSONError(e.name, errors.Join(ErrConversion, err))
	}
	return JSONResult{Value: data, Defined: true}, len(data), nil
}

func preparedJSONError(name string, err error) error {
	return &Error{Processor: name, Phase: PhaseOutput, Cause: err}
}
