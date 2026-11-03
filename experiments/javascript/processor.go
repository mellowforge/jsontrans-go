// Package javascript contains experimental assessment processors, not a supported
// JavaScript sandbox. See README.md and the gate report before using this code.
package javascript

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/experiments/javascript/internal/quickjswasm"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

type Backend string

const (
	QuickJS      Backend = "quickjs"
	Goja         Backend = "goja"
	OwnedQuickJS Backend = "owned-quickjs"
)

type Config struct {
	Limits           jt.Limits
	GuestMemoryBytes int
	GuestStackBytes  int
}
type Engine struct {
	backend Backend
	config  Config
	dir     string
	owned   *quickjswasm.Engine
}
type Program struct {
	engine      *Engine
	quick       []byte
	quickBridge []byte
	goja        any
}

//go:embed bridge.js
var bridge string

func New(backend Backend, c Config) (*Engine, error) {
	if backend != QuickJS && backend != Goja && backend != OwnedQuickJS {
		return nil, fmt.Errorf("unknown backend %q", backend)
	}
	if c.Limits.MaxSteps != 0 || c.Limits.RegexTimeout != 0 {
		return nil, fmt.Errorf("%w: JS instruction and per-regex limits", jt.ErrUnsupported)
	}
	if backend == Goja && (c.GuestMemoryBytes != 0 || c.GuestStackBytes != 0) {
		return nil, fmt.Errorf("%w: Goja byte memory/stack quotas", jt.ErrUnsupported)
	}
	l, err := budget.Resolve(jt.DefaultLimits(), &c.Limits)
	if err != nil {
		return nil, err
	}
	c.Limits = l
	if c.GuestMemoryBytes == 0 {
		c.GuestMemoryBytes = 64 << 20
	}
	if c.GuestStackBytes == 0 && backend != OwnedQuickJS {
		c.GuestStackBytes = 1 << 20
	}
	if c.GuestMemoryBytes < 0 || c.GuestStackBytes < 0 {
		return nil, jt.ErrLimit
	}
	if backend == OwnedQuickJS {
		if c.GuestMemoryBytes%65536 != 0 || c.GuestMemoryBytes < 2<<20 || c.GuestMemoryBytes > 64<<20 || c.GuestStackBytes != 0 {
			return nil, fmt.Errorf("%w: owned guest memory 2..64 MiB in pages; explicit JS stack quotas unsupported", jt.ErrUnsupported)
		}
		owned, err := quickjswasm.New(context.Background(), quickjswasm.Config{MemoryPages: uint32(c.GuestMemoryBytes / 65536), Timeout: l.Timeout})
		if err != nil {
			return nil, err
		}
		return &Engine{backend: backend, config: c, owned: owned}, nil
	}
	dir, err := os.MkdirTemp("", "jsontrans-js-empty-")
	if err != nil {
		return nil, err
	}
	return &Engine{backend: backend, config: c, dir: dir}, nil
}

// Close releases owned runtime resources, or removes the baseline wrapper mount.
// Baseline-wrapper calls must have finished first.
func (e *Engine) Close() error {
	if e.owned != nil {
		return e.owned.Close()
	}
	return os.Remove(e.dir)
}
func (e *Engine) Limits() jt.Limits  { return e.config.Limits }
func (p *Program) Limits() jt.Limits { return p.engine.Limits() }
func (e *Engine) Compile(ctx context.Context, expr string) (jt.Program, error) {
	b, err := budget.New(ctx, e.config.Limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if err = b.Expression(expr); err != nil {
		return nil, err
	}
	p := &Program{engine: e}
	source := "(" + expr + "\n)"
	if e.backend == OwnedQuickJS {
		p.quick, err = e.owned.Compile(b.Context(), source)
		if err == nil {
			p.quickBridge, err = e.owned.Compile(b.Context(), bridge)
		}
	} else if e.backend == Goja {
		p.goja, err = compileGoja(source)
	} else {
		p.quick, p.quickBridge, err = e.compileQuick(b.Context(), source)
	}
	if err == nil {
		err = b.Check()
	}
	if err != nil {
		return nil, diagnostic(e.backend, jt.PhaseCompile, err)
	}
	return p, nil
}
func diagnostic(backend Backend, phase string, err error) error {
	if err == nil {
		return nil
	}
	code := ""
	var cause error = err
	if strings.Contains(err.Error(), "JSONTRANS_LIMIT:") {
		cause = errors.Join(jt.ErrLimit, err)
	}
	if strings.Contains(err.Error(), "JSONTRANS_CONVERSION:") {
		cause = errors.Join(jt.ErrConversion, err)
	}
	if phase == jt.PhaseCompile && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		code = "SyntaxError"
	}
	return &jt.Error{Processor: string(backend), Phase: phase, Code: code, Cause: cause}
}
func (p *Program) Eval(ctx context.Context, input any, o jt.EvalOptions) (result jt.Result, err error) {
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
		return result, err
	}
	bindings := o.Bindings
	if bindings == nil {
		bindings = map[string]any{}
	}
	vars, err := encodeInput(bindings, b)
	if err != nil {
		return result, err
	}
	var text string
	if p.engine.backend == OwnedQuickJS {
		text, err = p.engine.owned.Eval(b.Context(), p.quick, p.quickBridge, in, vars, quickjswasm.Limits{Depth: l.MaxDepth, Items: l.MaxCollectionItems, Bytes: l.MaxOutputBytes, Work: l.MaxSteps})
	} else if p.engine.backend == Goja {
		text, err = p.runGoja(b.Context(), in, vars, l)
	} else {
		text, err = p.runQuick(b.Context(), in, vars, l)
	}
	if b.Check() != nil {
		return result, b.Check()
	}
	if err != nil {
		return result, diagnostic(p.engine.backend, jt.PhaseEvaluate, err)
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
		return result, errors.Join(jt.ErrConversion, err)
	}
	v, _, err = jsonvalue.CloneDetachedSize(v, b, l.MaxOutputBytes)
	if err != nil {
		return result, errors.Join(jt.ErrConversion, err)
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
