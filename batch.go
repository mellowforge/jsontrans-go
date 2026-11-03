package jsontrans

import (
	"context"
	"errors"
	"fmt"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

// Batch is an immutable ordered snapshot of compiled programs. It is safe for
// concurrent evaluations with separate destinations and caller-owned bindings.
type Batch struct {
	entries     []preparedEntry
	plan        prepared.BatchPlan
	unsupported LimitSet
}

// BatchError identifies the first failing expression using a zero-based index,
// including a deadline or cancellation noticed while that expression ran.
// Failures not tied to an expression, such as invalid options or document
// limits, are returned unwrapped. Cause preserves native diagnostics and
// supports errors.Is and errors.As.
type BatchError struct {
	Index int
	Cause error
}

func (e *BatchError) Error() string { return fmt.Sprintf("batch expression %d: %v", e.Index, e.Cause) }
func (e *BatchError) Unwrap() error { return e.Cause }

// NewBatch snapshots programs. Nil programs are rejected; an empty slice is valid.
// Construction is bounded by the default timeout and maximum collection size.
func NewBatch(programs []Program) (*Batch, error) {
	scope, err := budget.New(context.Background(), DefaultLimits())
	if err != nil {
		return nil, err
	}
	defer scope.Close()
	return newBatch(scope.Context(), programs)
}
func newBatch(ctx context.Context, programs []Program) (*Batch, error) {
	if len(programs) > DefaultLimits().MaxCollectionItems {
		return nil, fmt.Errorf("%w: batch program count", ErrLimit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := &Batch{entries: make([]preparedEntry, len(programs))}
	handlers := make([]*prepared.Handler, len(programs))
	for i, p := range programs {
		if err := ctx.Err(); err != nil {
			return nil, &BatchError{Index: i, Cause: err}
		}
		if nilValue(p) {
			return nil, &BatchError{Index: i, Cause: errors.New("nil batch program")}
		}
		entry, err := newPreparedEntry(p)
		if err != nil {
			return nil, &BatchError{Index: i, Cause: err}
		}
		b.entries[i] = entry
		b.unsupported |= entry.unsupported
		handlers[i] = entry.native
	}
	b.plan = prepared.CompileBatch(ctx, handlers)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b, nil
}

// CompileBatch compiles expressions in order, failing at the first compile error.
// The processor's configured timeout bounds the entire compilation. Constructors
// reject counts exceeding DefaultLimits().MaxCollectionItems before allocation.
func (x *Transformer) CompileBatch(ctx context.Context, processorName string, expressions []string) (*Batch, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if x == nil {
		return nil, errors.New("nil transformer")
	}
	if len(expressions) > DefaultLimits().MaxCollectionItems {
		return nil, fmt.Errorf("%w: batch program count", ErrLimit)
	}
	processor, ok := x.processors[processorName]
	if !ok {
		return nil, diagnostic(processorName, PhaseCompile, ErrUnknownProcessor)
	}
	limits, err := effectiveLimits(processor, nil)
	if err != nil {
		return nil, diagnostic(processorName, PhaseCompile, err)
	}
	scope, err := budget.New(ctx, limits)
	if err != nil {
		return nil, diagnostic(processorName, PhaseCompile, err)
	}
	defer scope.Close()
	programs := make([]Program, len(expressions))
	for i, expression := range expressions {
		if err = scope.Step(); err != nil {
			return nil, &BatchError{Index: i, Cause: diagnostic(processorName, PhaseCompile, err)}
		}
		p, err := x.Compile(scope.Context(), processorName, expression)
		if err != nil {
			return nil, &BatchError{Index: i, Cause: err}
		}
		programs[i] = p
	}
	batch, err := newBatch(scope.Context(), programs)
	if err != nil {
		return nil, diagnostic(processorName, PhaseCompile, err)
	}
	return batch, nil
}

// EvalPrepared evaluates sequentially and reuses dst capacity when possible.
// Every result owns its mutable data. Bindings must not alias dst. On failure it
// returns nil and clears populated slots; other dst contents are unspecified.
// A default one-second outer timeout and aggregate count/output limits apply in
// addition to independent configured limits and counters for every expression.
// Outer input dimensions apply even to empty batches and looser engine defaults.
func (b *Batch) EvalPrepared(ctx context.Context, d *Document, dst []Result, o EvalOptions) ([]Result, error) {
	if b == nil || d == nil || ctx == nil {
		return nil, errors.New("nil batch, document, or context")
	}
	limits, err := budget.Resolve(DefaultLimits(), o.Limits)
	if err != nil {
		return nil, err
	}
	var outer budget.Budget
	if err = budget.InitFastResolved(&outer, ctx, limits); err != nil {
		return nil, err
	}
	defer outer.Close()
	if err = outer.Collection(len(b.entries)); err != nil {
		return nil, err
	}
	if b.unsupported&specifiedLimits(o.Limits) != 0 {
		for i := range b.entries {
			entry := &b.entries[i]
			if err = outer.Check(); err != nil {
				return nil, err
			}
			if err = rejectUnsupportedLimits(entry.unsupported, o.Limits); err != nil {
				if entry.name != "" {
					err = diagnostic(entry.name, PhaseEvaluate, err)
				}
				return nil, &BatchError{Index: i, Cause: err}
			}
		}
	}
	if err = d.CheckLimits(outer.Context(), limits); err != nil {
		return nil, err
	}
	count := len(b.entries)
	if cap(dst) < count {
		dst = make([]Result, count)
	} else {
		dst = dst[:count]
	}
	if count == 0 {
		return dst, nil
	}
	var state any
	if b.plan != nil {
		state = b.plan.NewState(d)
	}
	bytes := 0
	var nativeScratch budget.Budget
	// With an override, adjacent entries that share configured limits reuse one
	// resolved value. This single-slot cache keeps heterogeneous batches at no
	// more than one Resolve per entry, with no map or allocation.
	var previousBase, resolved Limits
	haveResolved := false
	for i := range b.entries {
		entry := &b.entries[i]
		if err = outer.Check(); err != nil {
			return batchFailure(dst, i, err)
		}
		l := &entry.configured
		if o.Limits != nil {
			if !haveResolved || previousBase != *l {
				resolved, err = budget.Resolve(*l, o.Limits)
				if err != nil {
					// Resolve fails only for negative override fields, which the
					// outer Resolve above already rejects. Wrap as preparedEntry.eval
					// does in case that ever changes.
					if entry.name != "" {
						err = diagnostic(entry.name, PhaseEvaluate, err)
					}
					return batchFailure(dst, i, err)
				}
				previousBase, haveResolved = *l, true
			}
			l = &resolved
		}
		result, size, err := entry.evalResolved(outer.Context(), d, o, l, b.plan, state, i, &nativeScratch)
		if err != nil {
			return batchFailure(dst, i, err)
		}
		if size > limits.MaxOutputBytes-bytes {
			return batchFailure(dst, i, fmt.Errorf("%w: aggregate batch output size", ErrLimit))
		}
		bytes += size
		dst[i] = result
	}
	// Each expression checks the overall deadline before it starts and when it
	// returns, so cancellation is attributed to the expression that was running.
	return dst, nil
}

// batchFailure clears the results already stored for expressions before i and
// reports expression i as the failing one.
func batchFailure(dst []Result, i int, cause error) ([]Result, error) {
	clear(dst[:i])
	return nil, &BatchError{Index: i, Cause: cause}
}
