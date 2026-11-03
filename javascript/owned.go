package javascript

import (
	"context"
	"errors"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/javascript/internal/quickjswasm"
)

// ErrClosed identifies operations attempted on or interrupted by a closing engine.
var ErrClosed = quickjswasm.ErrClosed

// New initializes the optional JavaScript processor. ctx bounds initialization
// only; the caller owns the returned engine and must Close it.
func New(ctx context.Context, c Config) (*Engine, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	return newEngine(ctx, c)
}
func (e *Engine) Compile(ctx context.Context, expr string) (program jt.Program, err error) {
	ctx, done, err := e.lifetime.begin(ctx)
	if err != nil {
		return nil, ownedDiagnostic(jt.PhaseCompile, err)
	}
	defer done()
	program, err = e.compile(ctx, expr)
	err = ownedDiagnostic(jt.PhaseCompile, e.lifetime.failure(ctx, err))
	if err != nil {
		mapOwnedDiagnostic(expr, err)
		return nil, err
	}
	return program, nil
}
func (p *Program) Eval(ctx context.Context, input any, o jt.EvalOptions) (result jt.Result, err error) {
	ctx, done, err := p.engine.lifetime.begin(ctx)
	if err != nil {
		return result, ownedDiagnostic(jt.PhaseEvaluate, err)
	}
	defer done()
	result, err = p.eval(ctx, input, o)
	err = ownedDiagnostic(jt.PhaseEvaluate, p.engine.lifetime.failure(ctx, err))
	if err != nil {
		mapOwnedDiagnostic(p.source, err)
		return jt.Result{}, err
	}
	return result, nil
}
