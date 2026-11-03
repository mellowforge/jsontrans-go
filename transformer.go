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

// Limits bound compilation and evaluation work. Engine options set defaults;
// EvalOptions.Limits replaces each nonzero field for one evaluation.
type Limits = budget.Limits

// UTF16String holds a string containing isolated UTF-16 surrogate units as
// WTF-8 and preserves their JSON escapes through MarshalJSON. It appears in
// results and, after JSON decoding or document materialization, in the input
// and bindings processors receive.
type UTF16String = jsonvalue.UTF16String

// UTF16Object holds an object whose keys contain isolated surrogate units, in
// results and processor input. Ordinary objects remain map[string]any.
type UTF16Object = jsonvalue.UTF16Object

// DefaultLimits returns the library defaults used for zero limit fields.
func DefaultLimits() Limits { return budget.DefaultLimits() }

// Sentinel errors, matched with errors.Is through wrapping diagnostics.
var (
	ErrUnknownProcessor = errors.New("unknown processor")
	ErrInvalidInput     = jsonvalue.ErrInvalid
	ErrLimit            = budget.ErrLimit
	ErrUnsupported      = errors.New("unsupported option")
	ErrConversion       = errors.New("result cannot be represented as JSON")
)

// Processor compiles expressions of one language. A Transformer may call
// Compile concurrently.
type Processor interface {
	Compile(context.Context, string) (Program, error)
}

// Program is a compiled expression. Programs must be immutable and safe for
// concurrent Eval calls, and must not mutate input or bindings. Programs may
// implement EvalLimitSupport to declare unsupported evaluation limit fields.
type Program interface {
	Eval(context.Context, any, EvalOptions) (Result, error)
}

// LimitsProvider optionally supplies configured defaults to common wrappers.
// Third-party processors without it receive the library defaults.
type LimitsProvider interface{ Limits() Limits }

// EvalOptions configures one evaluation.
type EvalOptions struct {
	// Bindings are JSON-shaped variables visible to the expression. Processors
	// without variables reject them with ErrUnsupported.
	Bindings map[string]any
	// Limits replaces each nonzero field of the program's configured limits.
	Limits *Limits
}

// Result is a JSON-shaped evaluation result. Defined is false for a missing
// result; JSON null is Defined with a nil Value. A failed call returns the
// zero Result.
type Result struct {
	Value   any
	Defined bool
}

// JSONResult is an encoded Result: no bytes when missing, null bytes for null.
type JSONResult struct {
	Value   json.RawMessage
	Defined bool
}

// Phases reported in Error.Phase.
const (
	PhaseCompile  = "compile"
	PhaseEvaluate = "evaluate"
	PhaseInput    = "input"    // decoding or converting input
	PhaseBindings = "bindings" // converting evaluation bindings
	PhaseOutput   = "output"   // converting or encoding a result
)

// Error retains native diagnostic information and wraps the original cause.
type Error struct {
	// Processor names the reporting engine, or the registry name when the
	// engine did not name itself.
	Processor string
	// Phase is one of the Phase constants.
	Phase string
	// Code is the language's native error code, such as JSONata "S0202" or
	// JSONPath "syntax"; it is empty when the language defines none.
	Code string
	// Position is the native source position: UTF-16 code units for JSONata,
	// as the reference reports, and bytes for JSONPath. It is zero when the
	// failure has no source position.
	Position int
	// Token is the offending token or function name, when known.
	Token string
	Cause error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %s [%s] at %d: %v", e.Processor, e.Phase, e.Code, e.Position, e.Cause)
}
func (e *Error) Unwrap() error { return e.Cause }

// Transformer is an immutable registry of named processors, safe for
// concurrent use. It keeps no expression cache.
type Transformer struct{ processors map[string]Processor }

// New registers processors by name. The map is copied, so later changes to it
// do not affect the Transformer.
func New(processors map[string]Processor) (*Transformer, error) {
	x := &Transformer{processors: make(map[string]Processor, len(processors))}
	for name, p := range processors {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || nilValue(p) {
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

// Compile compiles expression with the named processor under its configured
// limits. Errors are *Error diagnostics attributed to that processor.
func (x *Transformer) Compile(ctx context.Context, name, expression string) (Program, error) {
	if x == nil {
		return nil, errors.New("nil transformer")
	}
	program, err := x.compile(ctx, name, expression)
	if err != nil {
		return nil, diagnostic(name, PhaseCompile, err)
	}
	return program, nil
}
func (x *Transformer) compile(ctx context.Context, name, expression string) (Program, error) {
	p, ok := x.processors[name]
	if !ok {
		return nil, ErrUnknownProcessor
	}
	l, err := effectiveLimits(p, nil)
	if err != nil {
		return nil, err
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if err = b.Expression(expression); err != nil {
		return nil, err
	}
	program, err := p.Compile(b.Context(), expression)
	if err != nil {
		return nil, err
	}
	if err = b.Check(); err != nil {
		return nil, err
	}
	if nilValue(program) {
		return nil, errors.New("processor returned nil program")
	}
	return newCheckedProgram(name, program, l)
}

// Process compiles and evaluates expression in one call. It compiles on every
// call; use Compile for repeated expressions.
func (x *Transformer) Process(ctx context.Context, name string, input any, expression string, options EvalOptions) (Result, error) {
	p, err := x.Compile(ctx, name, expression)
	if err != nil {
		return Result{}, err
	}
	return p.Eval(ctx, input, options)
}

// ProcessJSON is Process for JSON bytes; see EvalJSON.
func (x *Transformer) ProcessJSON(ctx context.Context, name string, input []byte, expression string, options EvalOptions) (JSONResult, error) {
	p, err := x.Compile(ctx, name, expression)
	if err != nil {
		return JSONResult{}, err
	}
	r, err := EvalJSON(ctx, p, input, options)
	return r, diagnostic(name, PhaseEvaluate, err)
}

// EvalJSON decodes input, evaluates p and encodes the result. Serialized object
// member order is kept for processors that enumerate keys. Invalid input is an
// *Error with Phase "input" and no processor; an unencodable result wraps
// ErrConversion.
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
	unsupported, err := unsupportedEvalLimits(p)
	if err == nil {
		err = rejectUnsupportedLimits(unsupported, options.Limits)
	}
	if err != nil {
		name := ""
		if checked, ok := p.(*checkedProgram); ok {
			name = checked.name
		}
		return JSONResult{}, diagnostic(name, PhaseEvaluate, err)
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return JSONResult{}, err
	}
	defer b.Close()
	v, err := jsonvalue.Decode(input, b)
	if err != nil {
		return JSONResult{}, &Error{Phase: PhaseInput, Cause: err}
	}
	if options.Bindings != nil {
		options.Bindings, err = jsonvalue.CloneBindings(options.Bindings, b, b.Limits.MaxInputBytes)
		if err != nil {
			return JSONResult{}, &Error{Phase: PhaseBindings, Cause: err}
		}
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
		return JSONResult{}, &Error{Phase: PhaseOutput, Cause: fmt.Errorf("%w: %w", ErrConversion, err)}
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
	name        string
	program     Program
	configured  Limits
	base        Limits
	customized  bool
	unsupported LimitSet
}

func (p *checkedProgram) Limits() Limits                  { return p.configured }
func (p *checkedProgram) UnsupportedEvalLimits() LimitSet { return p.unsupported }

// newCheckedProgram wraps a registry program. Programs are immutable, so the
// base forwarded to the program is fixed here, as on the prepared paths: the
// stricter of the wrapper and program ceilings with the program's regex
// timeout. customized records whether either configuration is non-default.
func newCheckedProgram(name string, program Program, configured Limits) (*checkedProgram, error) {
	unsupported, err := unsupportedEvalLimits(program)
	if err != nil {
		return nil, err
	}
	p := &checkedProgram{name: name, program: program, configured: configured, base: configured, customized: configured != DefaultLimits(), unsupported: unsupported}
	if _, ok := program.(LimitsProvider); ok {
		native, err := effectiveLimits(program, nil)
		if err != nil {
			return nil, err
		}
		p.base = intersectNativeLimits(configured, native)
		p.customized = p.customized || native != DefaultLimits()
	}
	return p, nil
}

// programLimits returns the limits forwarded to the wrapped program: the base
// resolved with any per-call override. It returns nil when every
// configuration is the default, so engine fast paths stay available.
func (p *checkedProgram) programLimits(override *Limits) (*Limits, error) {
	if override == nil && !p.customized {
		return nil, nil
	}
	l, err := budget.Resolve(p.base, override)
	if err != nil {
		return nil, err
	}
	l = supportedEvalLimits(l, p.unsupported)
	return &l, nil
}

func (p *checkedProgram) Eval(ctx context.Context, input any, options EvalOptions) (Result, error) {
	l, err := effectiveLimits(p, options.Limits)
	if err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	if err = rejectUnsupportedLimits(p.unsupported, options.Limits); err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	defer b.Close()
	v, err := jsonvalue.Clone(input, b, b.Limits.MaxInputBytes)
	if err != nil {
		return Result{}, diagnostic(p.name, PhaseInput, err)
	}
	if options.Bindings != nil {
		options.Bindings, err = jsonvalue.CloneBindings(options.Bindings, b, b.Limits.MaxInputBytes)
		if err != nil {
			return Result{}, diagnostic(p.name, PhaseBindings, err)
		}
	}
	if options.Limits, err = p.programLimits(options.Limits); err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	r, err := p.program.Eval(b.Context(), v, options)
	if err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	if err = b.Check(); err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	if !r.Defined {
		return Result{}, nil
	}
	r.Value, err = jsonvalue.Clone(r.Value, b, b.Limits.MaxOutputBytes)
	if err != nil {
		return Result{}, diagnostic(p.name, PhaseOutput, fmt.Errorf("%w: %w", ErrConversion, err))
	}
	if err = b.Check(); err != nil {
		return Result{}, diagnostic(p.name, PhaseEvaluate, err)
	}
	return r, nil
}
