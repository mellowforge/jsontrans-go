package javascript

import (
	"context"
	jt "github.com/mellowforge/jsontrans-go"
	publicjs "github.com/mellowforge/jsontrans-go/javascript"
)

// Assessment compatibility aliases; applications import the optional module.
type Exception = publicjs.Exception
type SourceLocation = publicjs.SourceLocation

var ErrClosed = publicjs.ErrClosed

// NewOwned forwards to the canonical optional processor module.
func NewOwned(ctx context.Context, c Config) (*Engine, error) {
	owned, err := publicjs.New(ctx, c)
	if err != nil {
		return nil, err
	}
	c.Limits = owned.Limits()
	if c.GuestMemoryBytes == 0 {
		c.GuestMemoryBytes = 64 << 20
	}
	return &Engine{backend: OwnedQuickJS, config: c, owned: owned}, nil
}
func (e *Engine) Compile(ctx context.Context, expr string) (jt.Program, error) {
	if e.owned != nil {
		return e.owned.Compile(ctx, expr)
	}
	return e.compile(ctx, expr)
}
func (p *Program) Eval(ctx context.Context, input any, o jt.EvalOptions) (jt.Result, error) {
	return p.eval(ctx, input, o)
}
