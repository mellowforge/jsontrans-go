// Package javascript provides the optional, CGO-free QuickJS/Wazero processor.
// It implements the documented restricted synchronous execution profile.
package javascript

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/javascript/internal/quickjswasm"
)

// Config snapshots per-engine defaults. See README.md for the execution profile.
type Config struct {
	// Limits configures compilation, conversion and supported evaluation controls.
	Limits jt.Limits
	// GuestMemoryBytes is a 64 KiB multiple from 2 to 64 MiB; zero selects 64 MiB.
	GuestMemoryBytes int
	// GuestStackBytes is reserved; explicit nonzero stack quotas are unsupported.
	GuestStackBytes int
}

// Engine owns its immutable compiled artifacts and runtime; callers must Close it.
type Engine struct {
	config   Config
	owned    *quickjswasm.Engine
	lifetime *engineLifetime
}

// Program is immutable and safe for concurrent evaluations while its engine is open.
type Program struct {
	engine      *Engine
	quick       []byte
	quickBridge []byte
	source      string
}

//go:embed bridge.js
var bridge string

func newEngine(ctx context.Context, c Config) (*Engine, error) {
	if c.Limits.MaxSteps != 0 || c.Limits.RegexTimeout != 0 {
		return nil, fmt.Errorf("%w: JS instruction and per-regex limits", jt.ErrUnsupported)
	}
	l, err := budget.Resolve(jt.DefaultLimits(), &c.Limits)
	if err != nil {
		return nil, err
	}
	c.Limits = l
	if c.GuestMemoryBytes == 0 {
		c.GuestMemoryBytes = 64 << 20
	}
	if c.GuestMemoryBytes < 0 || c.GuestStackBytes < 0 {
		return nil, jt.ErrLimit
	}
	if c.GuestMemoryBytes%65536 != 0 || c.GuestMemoryBytes < 2<<20 || c.GuestMemoryBytes > 64<<20 || c.GuestStackBytes != 0 {
		return nil, fmt.Errorf("%w: guest memory 2..64 MiB in pages; explicit JS stack quotas unsupported", jt.ErrUnsupported)
	}
	owned, err := quickjswasm.New(ctx, quickjswasm.Config{MemoryPages: uint32(c.GuestMemoryBytes / 65536), Timeout: l.Timeout})
	if err != nil {
		return nil, err
	}
	return &Engine{config: c, owned: owned, lifetime: newEngineLifetime()}, nil
}

// Close stops admission, cancels and drains complete calls, then releases resources.
func (e *Engine) Close() error                        { return e.lifetime.close(e.owned.Close) }
func (e *Engine) Limits() jt.Limits                   { return e.config.Limits }
func (p *Program) Limits() jt.Limits                  { return p.engine.Limits() }
func (p *Program) UnsupportedEvalLimits() jt.LimitSet { return jt.LimitSteps | jt.LimitRegexTimeout }
func (e *Engine) compile(ctx context.Context, expr string) (jt.Program, error) {
	b, err := budget.New(ctx, e.config.Limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if err = b.Expression(expr); err != nil {
		return nil, err
	}
	p := &Program{engine: e}
	p.source = expr
	source := "(\n" + expr + "\n)"
	p.quick, err = e.owned.CompileNamed(b.Context(), source, "rule.js")
	if err == nil {
		p.quickBridge, err = e.owned.CompileNamed(b.Context(), bridge, "bridge.js")
	}
	if err == nil {
		err = b.Check()
	}
	if err != nil {
		return nil, ownedDiagnostic(jt.PhaseCompile, err)
	}
	return p, nil
}
func (p *Program) eval(ctx context.Context, input any, o jt.EvalOptions) (result jt.Result, err error) {
	if o.Limits != nil && (o.Limits.MaxSteps != 0 || o.Limits.RegexTimeout != 0) {
		return result, fmt.Errorf("%w: JS instruction and per-regex limits", jt.ErrUnsupported)
	}
	l, err := budget.Resolve(p.engine.config.Limits, o.Limits)
	if err != nil {
		return result, err
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return result, err
	}
	defer b.Close()
	in, err := encodeInput(input, b)
	if err != nil {
		return result, ownedDiagnostic(jt.PhaseInput, err)
	}
	bindings := o.Bindings
	if bindings == nil {
		bindings = map[string]any{}
	}
	vars, err := encodeInput(bindings, b)
	if err != nil {
		return result, ownedDiagnostic(jt.PhaseBindings, err)
	}
	text, err := p.engine.owned.Eval(b.Context(), p.quick, p.quickBridge, in, vars, quickjswasm.Limits{Depth: l.MaxDepth, Items: l.MaxCollectionItems, Bytes: l.MaxOutputBytes, Work: l.MaxSteps, Timeout: l.Timeout})
	if b.Check() != nil {
		return result, b.Check()
	}
	if err != nil {
		return result, ownedDiagnostic(jt.PhaseEvaluate, err)
	}
	// The envelope is trusted bridge syntax; retain lossless scalar strings and numbers.
	raw := struct {
		Defined bool
		Value   json.RawMessage
	}{}
	if err = json.Unmarshal([]byte(text), &raw); err != nil {
		return result, fmt.Errorf("%w: bridge envelope: %w", jt.ErrConversion, err)
	}
	if !raw.Defined {
		return result, nil
	}
	inputLimit := b.Limits.MaxInputBytes
	b.Limits.MaxInputBytes = l.MaxOutputBytes
	v, err := jsonvalue.Decode(raw.Value, b)
	b.Limits.MaxInputBytes = inputLimit
	if err != nil {
		return result, ownedDiagnostic(jt.PhaseOutput, errors.Join(jt.ErrConversion, err))
	}
	v, _, err = jsonvalue.CloneDetachedSize(v, b, l.MaxOutputBytes)
	if err != nil {
		return result, ownedDiagnostic(jt.PhaseOutput, errors.Join(jt.ErrConversion, err))
	}
	if err = b.Check(); err != nil {
		return result, err
	}
	return jt.Result{Value: v, Defined: true}, nil
}
func encodeInput(v any, b *budget.Budget) (string, error) {
	owned, err := jsonvalue.Clone(v, b, b.Limits.MaxInputBytes)
	if err != nil {
		return "", err
	}
	var check func(any) error
	check = func(v any) error {
		if err := b.Enter(); err != nil {
			return err
		}
		defer b.Leave()
		switch x := v.(type) {
		case json.Number:
			n, err := x.Float64()
			if err != nil || math.IsInf(n, 0) {
				return fmt.Errorf("%w: binary64 overflow", jt.ErrInvalidInput)
			}
		case []any:
			for _, v := range x {
				if err := check(v); err != nil {
					return err
				}
			}
		case map[string]any:
			for _, v := range x {
				if err := check(v); err != nil {
					return err
				}
			}
		case jsonvalue.UTF16Object:
			return check(map[string]any(x))
		}
		return nil
	}
	if err = check(owned); err != nil {
		return "", err
	}
	data, err := json.Marshal(owned)
	if err != nil {
		return "", err
	}
	if err = b.Check(); err != nil {
		return "", err
	}
	return string(data), nil
}
