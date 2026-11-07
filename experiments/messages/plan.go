// Package messages contains development-only byte-to-byte experiment helpers.
// It exercises production APIs without changing processor behavior or defaults.
package messages

import (
	"context"
	"fmt"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/javascript"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

type Strategy string

const (
	Direct   Strategy = "direct"
	Prepared Strategy = "prepared"
	Batched  Strategy = "batch"
)

var Strategies = []Strategy{Direct, Prepared, Batched}

type Config struct {
	Engine      string
	Expressions []string
	Options     jt.EvalOptions
	Configured  bool
	Host        bool
}

// Plan owns immutable compiled state. Close after all its workers finish.
type Plan struct {
	programs []jt.Program
	batch    *jt.Batch
	options  jt.EvalOptions
	limits   jt.Limits
	closeFn  func() error
}

// Worker is local to a goroutine. The returned result slice is reused at its next
// Eval; JSON byte buffers themselves own storage and are never overwritten.
type Worker struct {
	plan *Plan
	out  []jt.JSONResult
}

func NewPlan(ctx context.Context, c Config) (p *Plan, err error) {
	l := jt.DefaultLimits()
	if c.Configured {
		l.MaxCollectionItems = 50000
	}
	l, err = budget.Resolve(l, c.Options.Limits)
	if err != nil {
		return nil, err
	}
	if c.Engine == "javascript" && c.Options.Limits != nil && (c.Options.Limits.MaxSteps != 0 || c.Options.Limits.RegexTimeout != 0) {
		return nil, jt.ErrUnsupported
	}
	setupLimits := l
	setupLimits.Timeout = 30 * time.Second
	scope, err := budget.New(ctx, setupLimits)
	if err != nil {
		return nil, err
	}
	defer scope.Close()
	if len(c.Expressions) == 0 {
		return nil, fmt.Errorf("at least one expression required")
	}
	if err = scope.Collection(len(c.Expressions)); err != nil {
		return nil, err
	}
	opts := c.Options
	if opts.Limits != nil {
		copied := *opts.Limits
		opts.Limits = &copied
	}
	if opts.Bindings != nil {
		opts.Bindings, err = jsonvalue.CloneBindings(opts.Bindings, scope, l.MaxInputBytes)
		if err != nil {
			return nil, err
		}
	}
	p = &Plan{limits: l, options: opts, closeFn: func() error { return nil }}
	defer func() {
		if err != nil {
			p.closeFn()
			p = nil
		}
	}()
	var engine jt.Processor
	base := jt.DefaultLimits()
	if c.Configured {
		base.MaxCollectionItems = 50000
	}
	switch c.Engine {
	case "jsonata":
		config := []jsonata.Option{jsonata.WithLimits(base)}
		if c.Host {
			config = append(config, jsonata.WithFunction("echo", jsonata.NativeFunction{N: 1, Fn: func(cc *jsonata.CallContext, v []jsonata.Value) (jsonata.Value, error) {
				if len(v) != 1 {
					return nil, fmt.Errorf("echo expects one argument")
				}
				if err := cc.Context.Err(); err != nil {
					return nil, err
				}
				return v[0], nil
			}}))
		}
		engine, err = jsonata.New(config...)
	case "jsonpath":
		config := []jsonpath.Option{jsonpath.WithLimits(base)}
		if c.Host {
			config = append(config, jsonpath.WithFunction("echo", jsonpath.Function{Parameters: []jsonpath.Type{jsonpath.ValueType}, Result: jsonpath.ValueType, Call: func(ctx context.Context, v []any) (any, error) {
				if len(v) != 1 {
					return nil, fmt.Errorf("echo expects one argument")
				}
				return v[0], ctx.Err()
			}}))
		}
		engine, err = jsonpath.New(config...)
	case "javascript":
		if c.Configured || c.Host {
			return p, fmt.Errorf("JavaScript baseline uses its approved default configuration")
		}
		var e *javascript.Engine
		e, err = javascript.New(scope.Context(), javascript.Config{Limits: jt.Limits{Timeout: 30 * time.Second}})
		if err == nil {
			engine = e
			p.closeFn = e.Close
			// Only startup/compilation gets a longer limit. Restore the agreed
			// message timeout through the supported per-call option.
			callLimits := jt.Limits{Timeout: l.Timeout}
			if p.options.Limits != nil {
				callLimits = *p.options.Limits
				if callLimits.Timeout == 0 {
					callLimits.Timeout = l.Timeout
				}
			}
			p.options.Limits = &callLimits
		}
	default:
		err = fmt.Errorf("unknown engine %q", c.Engine)
	}
	if err != nil {
		return p, err
	}
	registry, err := jt.New(map[string]jt.Processor{c.Engine: engine})
	if err != nil {
		return p, err
	}
	for _, expr := range c.Expressions {
		program, e := registry.Compile(scope.Context(), c.Engine, expr)
		if e != nil {
			return p, e
		}
		p.programs = append(p.programs, program)
	}
	p.batch, err = jt.NewBatch(p.programs)
	return p, err
}
func (p *Plan) Close() error { return p.closeFn() }
func (p *Plan) NewWorker() *Worker {
	return &Worker{plan: p, out: make([]jt.JSONResult, len(p.programs))}
}

func (w *Worker) Eval(ctx context.Context, raw []byte, s Strategy) (out []jt.JSONResult, err error) {
	if s != Direct && s != Prepared && s != Batched {
		return nil, fmt.Errorf("unknown strategy %q", s)
	}
	p := w.plan
	scope, err := budget.New(ctx, p.limits)
	if err != nil {
		return nil, err
	}
	defer scope.Close()
	clear(w.out)
	defer func() {
		if err != nil {
			clear(w.out)
			out = nil
		}
	}()
	if len(raw) > p.limits.MaxInputBytes {
		return nil, fmt.Errorf("%w: input bytes", jt.ErrLimit)
	}
	if err = scope.Collection(len(p.programs)); err != nil {
		return nil, err
	}
	var doc *jt.Document
	if s != Direct {
		doc, err = jt.PrepareJSON(scope.Context(), raw, jt.PrepareOptions{Limits: &p.limits})
		if err != nil {
			return nil, err
		}
		if s == Batched {
			var results []jt.JSONResult
			results, err = p.batch.EvalPreparedJSON(scope.Context(), doc, w.out, p.options)
			if err != nil {
				return nil, err
			}
			w.out = results
			if err = scope.Check(); err != nil {
				return nil, err
			}
			return w.out, nil
		}
	}
	used := 0
	for i, program := range p.programs {
		if err = scope.Check(); err != nil {
			return nil, err
		}
		var r jt.JSONResult
		if s == Direct {
			r, err = jt.EvalJSON(scope.Context(), program, raw, p.options)
		} else {
			r, err = jt.EvalPreparedJSON(scope.Context(), program, doc, p.options)
		}
		if err != nil {
			return nil, fmt.Errorf("expression %d: %w", i, err)
		}
		if len(r.Value) > p.limits.MaxOutputBytes-used {
			return nil, fmt.Errorf("%w: aggregate encoded bytes", jt.ErrLimit)
		}
		used += len(r.Value)
		w.out[i] = r
	}
	if err = scope.Check(); err != nil {
		return nil, err
	}
	return w.out, nil
}
