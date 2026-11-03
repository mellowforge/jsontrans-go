// Package prepared seals trusted native prepared evaluation to compiled owners.
// It has no registry or evaluation state shared between calls.
package prepared

import (
	"context"
	"errors"
	"reflect"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

// Request contains checked immutable input, owned bindings, and expression-local
// accounting. Customized disables native fast paths: callers set it for custom
// options, and a batch plan sets it when its own fast walk already ran.
type Request struct {
	Document        *document.Document
	Bindings        map[string]any
	Limits          budget.Limits
	Budget          *budget.Budget
	Customized      bool
	managedBoundary bool
}

// CheckEntry checks bare native calls. Requests created by Evaluate/EvaluateInto
// already have a checked entry and a mandatory successful-return check.
// This is only for handler boundaries; loop and resource checks remain required.
func (r Request) CheckEntry() error {
	if r.managedBoundary {
		return nil
	}
	return r.Budget.Check()
}

// CheckExit leaves the successful-return check to an enclosing evaluator, while
// preserving it for direct native handler calls.
func (r Request) CheckExit() error { return r.CheckEntry() }

// Result owns validated detached JSON. Bytes is its exact encoded JSON size.
type Result struct {
	Value   any
	Defined bool
	Bytes   int
}

// BatchPlan is immutable; state belongs to one batch evaluation only.
type BatchPlan interface {
	NewState(*document.Document) any
	Eval(index int, request Request, state any) (Result, error)
}
type BatchCompiler func(context.Context, []*Handler) (BatchPlan, bool)

// Handler cannot be forged or rebound by external adapters. In particular a
// promoted NativePrepared method does not authorize an embedding wrapper.
type Handler struct {
	owner    any
	eval     func(Request) (Result, error)
	compiler BatchCompiler
	reusable bool
}

func New(owner any, eval func(Request) (Result, error), compiler ...BatchCompiler) *Handler {
	if owner == nil || reflect.ValueOf(owner).Kind() != reflect.Pointer || reflect.ValueOf(owner).IsNil() || eval == nil {
		panic("invalid native prepared handler")
	}
	h := &Handler{owner: owner, eval: eval}
	if len(compiler) > 0 {
		h.compiler = compiler[0]
	}
	return h
}

// NewReusable additionally certifies that native evaluation cannot publish a
// runtime Budget or callback context. Callers must still disable reuse for
// custom options, bindings, instrumentation, and unsupported input dimensions.
func NewReusable(owner any, eval func(Request) (Result, error), compiler ...BatchCompiler) *Handler {
	h := New(owner, eval, compiler...)
	h.reusable = true
	return h
}
func (h *Handler) ReusableBudget() bool           { return h.reusable }
func (h *Handler) Owner() any                     { return h.owner }
func (h *Handler) Eval(r Request) (Result, error) { return h.eval(r) }

// Lookup checks exact owner identity, not interface method promotion.
func Lookup(candidate any) *Handler {
	p, ok := candidate.(interface{ NativePrepared() *Handler })
	if !ok {
		return nil
	}
	v := reflect.ValueOf(candidate)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil
	}
	h := p.NativePrepared()
	if h == nil || h.owner != candidate {
		return nil
	}
	return h
}

// CompileBatch tries the first native handler's compiler only if all entries are
// exact-owner native handlers. Compilers must reject incompatible handlers.
func CompileBatch(ctx context.Context, handlers []*Handler) BatchPlan {
	if len(handlers) == 0 {
		return nil
	}
	for _, h := range handlers {
		if h == nil {
			return nil
		}
	}
	if compiler := handlers[0].compiler; compiler != nil {
		if plan, ok := compiler(ctx, handlers); ok {
			return plan
		}
	}
	return nil
}

// Evaluate is the common direct/root native boundary. The callback receives a
// single checked scope and owned bindings, and must return a detached Result.
func Evaluate(ctx context.Context, d *document.Document, bindings map[string]any, limits budget.Limits, customized bool, eval func(Request) (Result, error)) (Result, error) {
	var local budget.Budget
	return evaluate(&local, ctx, d, bindings, limits, customized, eval, true)
}

// EvaluateInto reinitializes caller-owned scratch for one pure native expression.
// Neither eval nor any called runtime may retain the Budget or publish a callback
// context. Calls must be sequential, and each call owns and closes its scope.
func EvaluateInto(local *budget.Budget, ctx context.Context, d *document.Document, bindings map[string]any, limits budget.Limits, customized bool, eval func(Request) (Result, error)) (Result, error) {
	return evaluate(local, ctx, d, bindings, limits, customized, eval, false)
}
func evaluate(local *budget.Budget, ctx context.Context, d *document.Document, bindings map[string]any, limits budget.Limits, customized bool, eval func(Request) (Result, error), owned bool) (Result, error) {
	if d == nil {
		return Result{}, errors.New("nil document")
	}
	if err := d.CheckLimits(ctx, limits); err != nil {
		return Result{}, err
	}
	var err error
	if owned {
		err = budget.InitFastOwnedResolved(local, ctx, limits)
	} else {
		err = budget.InitFastResolved(local, ctx, limits)
	}
	if err != nil {
		return Result{}, err
	}
	defer local.Close()
	if bindings != nil {
		if bindings, err = jsonvalue.CloneBindings(bindings, local, limits.MaxInputBytes); err != nil {
			return Result{}, err
		}
	}
	result, err := eval(Request{Document: d, Bindings: bindings, Limits: limits, Budget: local, Customized: customized, managedBoundary: true})
	if err != nil {
		return Result{}, err
	}
	if err = local.Check(); err != nil {
		return Result{}, err
	}
	return result, nil
}
