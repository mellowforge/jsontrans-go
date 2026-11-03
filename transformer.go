// Package jsontrans provides extensible, context-aware JSON processing.
package jsontrans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/errchain"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

type Limits = budget.Limits

// UTF16String is used only for JSON result strings containing isolated UTF-16
// surrogate units. It preserves their JSON escapes through MarshalJSON.
type UTF16String = jsonvalue.UTF16String

// UTF16Object preserves the unusual JSON object keys containing isolated
// surrogate units. Ordinary objects remain map[string]any.
type UTF16Object = jsonvalue.UTF16Object

func DefaultLimits() Limits { return budget.DefaultLimits() }

var (
	ErrUnknownProcessor = errors.New("unknown processor")
	ErrInvalidInput     = jsonvalue.ErrInvalid
	ErrLimit            = budget.ErrLimit
	ErrUnsupported      = errors.New("unsupported option")
	ErrConversion       = errors.New("result cannot be represented as JSON")
)

type Processor interface {
	Compile(context.Context, string) (Program, error)
}
type Program interface {
	Eval(context.Context, any, EvalOptions) (Result, error)
}

// LimitsProvider optionally supplies configured defaults to common wrappers.
// Third-party processors without it receive the library defaults.
type LimitsProvider interface{ Limits() Limits }
type EvalOptions struct {
	Bindings map[string]any
	Limits   *Limits
}
type Result struct {
	Value   any
	Defined bool
}
type JSONResult struct {
	Value   json.RawMessage
	Defined bool
}

// Error retains native diagnostic information and wraps the original cause.
type Error struct {
	Processor string
	Phase     string
	Code      string
	Position  int
	Token     string
	Cause     error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %s [%s] at %d: %v", e.Processor, e.Phase, e.Code, e.Position, e.Cause)
}
func (e *Error) Unwrap() error { return e.Cause }

type Transformer struct{ processors map[string]Processor }

func New(processors map[string]Processor) (*Transformer, error) {
	x := &Transformer{processors: make(map[string]Processor, len(processors))}
	for name, p := range processors {
		if strings.TrimSpace(name) == "" || nilValue(p) {
			return nil, fmt.Errorf("invalid processor registration %q", name)
		}
		x.processors[name] = p
	}
	return x, nil
}

// diagnostic attributes err to the processor registered as name. A *Error the
// processor returns directly is its own (engines name themselves, which may
// differ from the registry name) and is completed rather than wrapped again.
// Deeper in a chain, only a diagnostic without a processor or with this name
// lends its fields to a wrapper that keeps the whole chain; other processors'
// diagnostics stay reachable as the cause.
func diagnostic(name, phase string, err error) error {
	if err == nil {
		return nil
	}
	native, direct := err.(*Error)
	if !direct {
		var ok bool
		native, ok = errchain.Find(err, func(e *Error) bool { return e.Processor == "" || e.Processor == name })
		if !ok {
			return &Error{Processor: name, Phase: phase, Cause: err}
		}
	}
	out := *native
	if out.Processor == "" {
		out.Processor = name
	}
	if out.Phase == "" {
		out.Phase = phase
	}
	if !direct {
		out.Cause = err
	}
	return &out
}

func (x *Transformer) Compile(ctx context.Context, name, expression string) (Program, error) {
	p, ok := x.processors[name]
	if !ok {
		return nil, diagnostic(name, "compile", ErrUnknownProcessor)
	}
	l, err := effectiveLimits(p, nil)
	if err != nil {
		return nil, diagnostic(name, "compile", err)
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return nil, diagnostic(name, "compile", err)
	}
	defer b.Close()
	if err = b.Expression(expression); err != nil {
		return nil, diagnostic(name, "compile", err)
	}
	program, err := p.Compile(b.Context(), expression)
	if err != nil {
		return nil, diagnostic(name, "compile", err)
	}
	if err = b.Check(); err != nil {
		return nil, diagnostic(name, "compile", err)
	}
	if nilValue(program) {
		return nil, diagnostic(name, "compile", errors.New("processor returned nil program"))
	}
	return &checkedProgram{name: name, program: program, configured: l}, nil
}

func (x *Transformer) Process(ctx context.Context, name string, input any, expression string, options EvalOptions) (Result, error) {
	p, err := x.Compile(ctx, name, expression)
	if err != nil {
		return Result{}, err
	}
	return p.Eval(ctx, input, options)
}
func (x *Transformer) ProcessJSON(ctx context.Context, name string, input []byte, expression string, options EvalOptions) (JSONResult, error) {
	p, err := x.Compile(ctx, name, expression)
	if err != nil {
		return JSONResult{}, err
	}
	r, err := EvalJSON(ctx, p, input, options)
	return r, diagnostic(name, "json", err)
}
func EvalJSON(ctx context.Context, p Program, input []byte, options EvalOptions) (JSONResult, error) {
	if nilValue(p) {
		return JSONResult{}, errors.New("nil program")
	}
	if ctx == nil {
		return JSONResult{}, errors.New("nil context")
	}
	ctx = jsonvalue.WithObjectOrder(ctx)
	l, err := effectiveLimits(p, options.Limits)
	if err != nil {
		return JSONResult{}, err
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return JSONResult{}, err
	}
	defer b.Close()
	v, err := jsonvalue.Decode(input, b)
	if err != nil {
		return JSONResult{}, &Error{Phase: "input", Cause: err}
	}
	if options.Bindings != nil {
		detached, err := jsonvalue.Clone(options.Bindings, b, b.Limits.MaxInputBytes)
		if err != nil {
			return JSONResult{}, &Error{Phase: "bindings", Cause: err}
		}
		options.Bindings = bindingsMap(detached)
	}
	r, err := p.Eval(b.Context(), v, options)
	if err != nil {
		return JSONResult{}, err
	}
	if err = b.Check(); err != nil {
		return JSONResult{}, err
	}
	if !r.Defined {
		return JSONResult{}, nil
	}
	data, err := jsonvalue.Encode(r.Value, b)
	if err != nil {
		return JSONResult{}, &Error{Phase: "output", Cause: fmt.Errorf("%w: %w", ErrConversion, err)}
	}
	return JSONResult{Value: data, Defined: true}, nil
}
func effectiveLimits(provider any, override *Limits) (Limits, error) {
	l := DefaultLimits()
	if p, ok := provider.(LimitsProvider); ok {
		configured := p.Limits()
		var err error
		l, err = budget.Resolve(l, &configured)
		if err != nil {
			return Limits{}, err
		}
	}
	return budget.Resolve(l, override)
}
func nilValue(v any) bool {
	if v == nil {
		return true
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(v).IsNil()
	}
	return false
}

type checkedProgram struct {
	name       string
	program    Program
	configured Limits
}

func (p *checkedProgram) Limits() Limits { return p.configured }

func (p *checkedProgram) Eval(ctx context.Context, input any, options EvalOptions) (Result, error) {
	l, err := effectiveLimits(p, options.Limits)
	if err != nil {
		return Result{}, diagnostic(p.name, "evaluate", err)
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return Result{}, diagnostic(p.name, "evaluate", err)
	}
	defer b.Close()
	v, err := jsonvalue.Clone(input, b, b.Limits.MaxInputBytes)
	if err != nil {
		return Result{}, diagnostic(p.name, "input", err)
	}
	if options.Bindings != nil {
		detached, err := jsonvalue.Clone(options.Bindings, b, b.Limits.MaxInputBytes)
		if err != nil {
			return Result{}, diagnostic(p.name, "bindings", err)
		}
		options.Bindings = bindingsMap(detached)
	}
	r, err := p.program.Eval(b.Context(), v, options)
	if err != nil {
		return Result{}, diagnostic(p.name, "evaluate", err)
	}
	if err = b.Check(); err != nil {
		return Result{}, diagnostic(p.name, "evaluate", err)
	}
	if !r.Defined {
		return Result{}, nil
	}
	r.Value, err = jsonvalue.Clone(r.Value, b, b.Limits.MaxOutputBytes)
	if err != nil {
		return Result{}, diagnostic(p.name, "output", fmt.Errorf("%w: %w", ErrConversion, err))
	}
	if err = b.Check(); err != nil {
		return Result{}, diagnostic(p.name, "evaluate", err)
	}
	return r, nil
}

func bindingsMap(v any) map[string]any {
	if object, ok := v.(UTF16Object); ok {
		return map[string]any(object)
	}
	return v.(map[string]any)
}
