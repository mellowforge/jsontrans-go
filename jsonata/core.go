// Package jsonata implements the JSONata expression language.
package jsonata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

type Engine struct {
	limits          jsontrans.Limits
	functions       map[string]Callable
	builtins        map[string]Callable
	evaluationDepth int
}
type Option func(*Engine) error

// WithEvaluationDepth enables a JSONata expression-frame guard; zero disables it.
func WithEvaluationDepth(n int) Option {
	return func(e *Engine) error {
		if n < 0 {
			return fmt.Errorf("%w: negative evaluation depth", jsontrans.ErrLimit)
		}
		e.evaluationDepth = n
		return nil
	}
}

func WithLimits(l jsontrans.Limits) Option {
	return func(e *Engine) error { var err error; e.limits, err = budget.Resolve(e.limits, &l); return err }
}
func WithFunction(name string, f Callable) Option {
	return func(e *Engine) error {
		if name == "" || f == nil {
			return fmt.Errorf("invalid function")
		}
		e.functions[strings.TrimPrefix(name, "$")] = f
		return nil
	}
}
func New(opts ...Option) (*Engine, error) {
	e := &Engine{limits: budget.DefaultLimits(), functions: map[string]Callable{}, builtins: withBuiltinSignatures(builtins())}
	for _, o := range opts {
		if o == nil {
			return nil, fmt.Errorf("nil option")
		}
		if err := o(e); err != nil {
			return nil, err
		}
	}
	return e, nil
}

type Program struct {
	root         *node
	engine       *Engine
	trackParents bool
	fast         *fastPlan
	prepared     *prepared.Handler
}

func (e *Engine) Compile(ctx context.Context, expr string) (jsontrans.Program, error) {
	return e.CompileRaw(ctx, expr)
}
func (e *Engine) CompileRaw(ctx context.Context, expr string) (*Program, error) {
	b, err := budget.New(ctx, e.limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if err = b.Expression(expr); err != nil {
		return nil, err
	}
	p := parser{s: expr, b: b}
	p.next()
	root := p.parse(0)
	if p.err == nil && p.t.kind != "eof" {
		p.fail("S0201")
	}
	if p.err != nil {
		return nil, convertCompilePosition(p.err, expr)
	}
	if err := convertNodePositions(root, expr, b); err != nil {
		return nil, err
	}
	snapshot := &Engine{limits: e.limits, functions: make(map[string]Callable, len(e.functions)), builtins: make(map[string]Callable, len(e.builtins)), evaluationDepth: e.evaluationDepth}
	for k, v := range e.builtins {
		snapshot.builtins[k] = v
	}
	for k, v := range e.functions {
		snapshot.functions[k] = v
	}
	program := &Program{root: root, engine: snapshot, trackParents: containsParent(root), fast: compileFastPlan(root)}
	newHandler := prepared.New
	if program.fast != nil && snapshot.evaluationDepth == 0 && len(snapshot.functions) == 0 {
		newHandler = prepared.NewReusable
	}
	program.prepared = newHandler(program, program.evalPrepared, compilePreparedBatch)
	return program, nil
}
func (p *Program) Eval(ctx context.Context, input any, opts jsontrans.EvalOptions) (jsontrans.Result, error) {
	v, err := p.evaluate(ctx, input, opts, false)
	if err != nil {
		return jsontrans.Result{}, p.wrapTimebox(err)
	}
	if IsUndefined(v) {
		return jsontrans.Result{}, nil
	}
	return jsontrans.Result{Value: v, Defined: true}, nil
}
func (p *Program) EvalRaw(ctx context.Context, input any, opts jsontrans.EvalOptions) (Value, error) {
	v, err := p.evaluate(ctx, input, opts, true)
	return v, p.wrapTimebox(err)
}
func (p *Program) evaluate(ctx context.Context, input any, opts jsontrans.EvalOptions, raw bool) (Value, error) {
	if !raw && IsUndefined(input) {
		return nil, fmt.Errorf("%w: undefined is not JSON", jsontrans.ErrInvalidInput)
	}
	limits, err := budget.Resolve(p.engine.limits, opts.Limits)
	if err != nil {
		return nil, err
	}
	b, err := budget.New(ctx, limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if !IsUndefined(input) {
		input, err = jsonvalue.Clone(input, b, b.Limits.MaxInputBytes)
		if err != nil {
			return nil, err
		}
		input = normalize(input)
		if err = validateFinite(input, b); err != nil {
			return nil, err
		}
	}
	var bindings map[string]any
	if opts.Bindings != nil {
		x, err := jsonvalue.Clone(opts.Bindings, b, b.Limits.MaxInputBytes)
		if err != nil {
			return nil, err
		}
		if object, ok := x.(jsontrans.UTF16Object); ok {
			bindings = map[string]any(object)
		} else {
			bindings = x.(map[string]any)
		}
	}
	v, _, err := p.evaluateOwned(input, bindings, b, raw, false)
	return v, err
}

// evaluateOwned consumes evaluation-local input and bindings without another
// scope or input copy. Only checked prepared dispatch uses detached results.
func (p *Program) evaluateOwned(input Value, bindings map[string]any, b *budget.Budget, raw, detached bool) (Value, int, error) {
	vars := map[string]Value{"$": input}
	if bindings != nil {
		x := normalize(bindings)
		if err := validateFinite(x, b); err != nil {
			return nil, 0, err
		}
		for k, v := range x.(map[string]any) {
			vars[strings.TrimPrefix(k, "$")] = v
		}
	}
	ev := evaluator{b: b, root: input, vars: vars, env: environment{builtins: p.engine.builtins, functions: p.engine.functions}, now: time.Now(), trackParents: p.trackParents, objectOrder: map[uintptr]objectOrderEntry{}, depth: newDepthTracker(p.engine.evaluationDepth)}
	v, err := ev.eval(p.root, input)
	if err != nil {
		return nil, 0, err
	}
	if raw || IsUndefined(v) {
		return v, 0, b.Check()
	}
	v, err = materializeChecked(v, b)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", jsontrans.ErrConversion, err)
	}
	size := 0
	if detached {
		v, size, err = jsonvalue.CloneDetachedSize(v, b, b.Limits.MaxOutputBytes)
	} else {
		v, err = jsonvalue.Clone(v, b, b.Limits.MaxOutputBytes)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", jsontrans.ErrConversion, err)
	}
	return v, size, b.Check()
}

func validateFinite(v any, b *budget.Budget) error {
	if err := b.Enter(); err != nil {
		return err
	}
	defer b.Leave()
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Errorf("%w: number out of binary64 range", jsontrans.ErrInvalidInput)
		}
	case []any:
		for _, v := range x {
			if err := validateFinite(v, b); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, v := range x {
			if err := validateFinite(v, b); err != nil {
				return err
			}
		}
	}
	return nil
}
func normalize(v any) any {
	switch x := v.(type) {
	case jsontrans.UTF16String:
		return string(x)
	case jsontrans.UTF16Object:
		return normalize(map[string]any(x))
	case json.Number:
		n, _ := x.Float64()
		return n
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case []any:
		for i := range x {
			x[i] = normalize(x[i])
		}
	case map[string]any:
		for k, v := range x {
			x[k] = normalize(v)
		}
	}
	return v
}

type token struct {
	kind, val string
	pos       int
	quoted    bool
}
type node struct {
	sig      *functionSignature
	op       string
	val      any
	pos      int
	children []*node
}
type parser struct {
	s   string
	i   int
	t   token
	err error
	b   *budget.Budget
}

func (p *parser) fail(code string) {
	if p.err == nil {
		p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: code, Position: p.t.pos, Token: p.t.val, Cause: fmt.Errorf("unexpected token %q", p.t.val)}
	}
}
func (p *parser) next() {
	if p.err != nil {
		return
	}
	if err := p.b.Step(); err != nil {
		p.err = err
		return
	}
	for p.i < len(p.s) {
		r, n := utf8.DecodeRuneInString(p.s[p.i:])
		if !unicode.IsSpace(r) {
			break
		}
		p.i += n
	}
	if p.i+1 < len(p.s) && p.s[p.i:p.i+2] == "/*" {
		end := strings.Index(p.s[p.i+2:], "*/")
		if end < 0 {
			p.t = token{kind: "comment", pos: p.i + 1}
			p.fail("S0106")
			return
		}
		p.i += end + 4
		p.next()
		return
	}
	start := p.i
	if start >= len(p.s) {
		p.t = token{kind: "eof", pos: start}
		return
	}
	c := p.s[p.i]
	first, size := utf8.DecodeRuneInString(p.s[p.i:])
	p.i += size
	if c == '"' || c == '\'' {
		for p.i < len(p.s) {
			if p.s[p.i] == '\\' {
				if p.i+1 >= len(p.s) {
					p.t = token{pos: p.i + 1}
					p.fail("S0101")
					return
				}
				escape := p.s[p.i+1]
				if escape == 'u' {
					if p.i+6 > len(p.s) {
						p.t = token{pos: p.i + 1}
						p.fail("S0104")
						return
					}
					for _, h := range p.s[p.i+2 : p.i+6] {
						if !strings.ContainsRune("0123456789abcdefABCDEF", h) {
							p.t = token{pos: p.i + 1}
							p.fail("S0104")
							return
						}
					}
					p.i += 6
					continue
				}
				if !strings.ContainsRune(`"\/bfnrt`, rune(escape)) && !(c == '\'' && escape == '\'') {
					p.t = token{pos: p.i + 1}
					p.fail("S0103")
					return
				}
				p.i += 2
				continue
			}
			if p.s[p.i] == c {
				p.i++
				raw := p.s[start:p.i]
				v, err := decodeStringLiteral(raw)
				if err != nil {
					p.t = token{pos: start + 1}
					p.fail("S0101")
					return
				}
				p.t = token{"string", v, p.i, false}
				return
			}
			p.i++
		}
		p.fail("S0101")
		return
	}
	if c >= '0' && c <= '9' {
		for p.i < len(p.s) && ((p.s[p.i] >= '0' && p.s[p.i] <= '9') || p.s[p.i] == '.' || p.s[p.i] == 'e' || p.s[p.i] == 'E' || ((p.s[p.i] == '+' || p.s[p.i] == '-') && (p.s[p.i-1] == 'e' || p.s[p.i-1] == 'E'))) {
			if p.s[p.i] == '.' && p.i+1 < len(p.s) && p.s[p.i+1] == '.' {
				break
			}
			p.i++
		}
		p.t = token{"number", p.s[start:p.i], p.i, false}
		return
	}
	if c == '$' || unicode.IsLetter(first) || c == '_' || c == '`' {
		if c == '`' {
			for p.i < len(p.s) && p.s[p.i] != '`' {
				p.i++
			}
			if p.i >= len(p.s) {
				p.fail("S0105")
				return
			}
			p.i++
			p.t = token{"name", p.s[start+1 : p.i-1], p.i, true}
			return
		}
		for p.i < len(p.s) {
			r, n := utf8.DecodeRuneInString(p.s[p.i:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
				break
			}
			p.i += n
		}
		s := p.s[start:p.i]
		kind := "name"
		if c == '$' {
			kind = "variable"
		}
		p.t = token{kind, s, p.i, false}
		return
	}
	if p.i < len(p.s) {
		two := p.s[start : p.i+1]
		switch two {
		case ":=", "!=", "<=", ">=", "..", "**", "~>", "??", "?:":
			p.i++
			p.t = token{two, two, p.i, false}
			return
		}
	}
	p.t = token{string(c), string(c), p.i, false}
}
func precedence(t token) int {
	// Literal text and quoted names are data, even when they spell an operator.
	if t.quoted {
		return 0
	}
	switch t.kind {
	case "string", "number", "variable", "eof":
		return 0
	}
	switch t.val {
	case ":=":
		return 1
	case "?", "?:", "??":
		return 2
	case "or":
		return 3
	case "and":
		return 4
	case "=", "!=", "<", ">", "<=", ">=", "in", "~>":
		return 5
	case "&":
		return 6
	case "+", "-", "..":
		return 7
	case "*", "/", "%":
		return 8
	case "^", "{", "#", "@":
		return 9
	case ".":
		return 10
	case "[", "(":
		return 11
	}
	return 0
}
func (p *parser) parse(min int) *node {
	if p.err != nil {
		return nil
	}
	if err := p.b.Enter(); err != nil {
		p.err = err
		return nil
	}
	defer p.b.Leave()
	t := p.t
	if t.kind != "/" {
		p.next()
	}
	n := &node{op: t.kind, val: t.val, pos: t.pos}
	switch t.kind {
	case "eof":
		p.fail("S0207")
		return n
	case "number":
		v, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			p.fail("S0102")
		}
		n.val = v
	case "string", "variable":
	case "?":
		n.op = "placeholder"
	case "%":
		n.op = "parent"
	case "name":
		if (t.val == "function" || t.val == "λ") && p.t.kind == "(" {
			p.expect("(")
			n.op = "lambda"
			n.children = p.list(")")
			for _, param := range n.children {
				if param.op != "variable" {
					p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: "S0208", Position: param.pos, Cause: fmt.Errorf("function parameter must be a variable")}
					return n
				}
			}
			if p.t.kind == "<" {
				start := p.t.pos - 1
				i := start
				depth := 0
				for i < len(p.s) {
					if p.s[i] == '<' {
						depth++
					}
					if p.s[i] == '>' {
						depth--
						if depth == 0 {
							break
						}
					}
					i++
				}
				if i >= len(p.s) {
					p.fail("S0401")
					return n
				}
				sig, err := parseSignature(p.s[start : i+1])
				if err != nil {
					var native *jsontrans.Error
					if errors.As(err, &native) {
						copy := *native
						copy.Position += start + 1
						err = &copy
					}
					p.err = err
					return n
				}
				n.sig = sig
				p.i = i + 1
				p.next()
			}
			p.expect("{")
			n.children = append(n.children, p.parse(0))
			p.expect("}")
			break
		}
		switch t.val {
		case "true":
			n.op = "literal"
			n.val = true
		case "false":
			n.op = "literal"
			n.val = false
		case "null":
			n.op = "literal"
			n.val = nil
		}
	case "|":
		n.op = "transform"
		n.children = append(n.children, p.parse(0))
		p.expect("|")
		n.children = append(n.children, p.parse(0))
		if p.t.kind == "," {
			p.next()
			n.children = append(n.children, p.parse(0))
		}
		p.expect("|")
	case "/":
		start := t.pos
		i := start
		inClass := false
		escaped := false
		for i < len(p.s) {
			c := p.s[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if c == '\\' {
				escaped = true
				i++
				continue
			}
			if c == '[' {
				inClass = true
			}
			if c == ']' {
				inClass = false
			}
			if c == '/' && !inClass {
				break
			}
			i++
		}
		if i >= len(p.s) {
			p.fail("S0302")
			return n
		}
		pattern := p.s[start:i]
		i++
		flagStart := i
		for i < len(p.s) && (p.s[i] == 'i' || p.s[i] == 'm') {
			i++
		}
		n.op = "regex"
		n.val = RegexMatcher{Pattern: pattern, Flags: p.s[flagStart:i]}
		p.i = i
		p.next()
	case "*", "**":
		n.op = "wild" + t.kind
	case "-":
		n.op = "neg"
		n.children = []*node{p.parse(9)}
	case "(":
		n.op = "block"
		for p.t.kind != ")" && p.err == nil {
			n.children = append(n.children, p.parse(0))
			if p.t.kind != ";" {
				break
			}
			p.next()
		}
		p.expect(")")
	case "[":
		n.op = "array"
		n.children = p.list("]")
	case "{":
		n.op = "object"
		for p.t.kind != "}" && p.err == nil {
			key := p.parse(0)
			p.expect(":")
			value := p.parse(0)
			n.children = append(n.children, key, value)
			if p.t.kind != "," {
				break
			}
			p.next()
		}
		p.expect("}")
	default:
		p.fail("S0211")
		return n
	}
	for p.err == nil {
		prec := precedence(p.t)
		if prec == 0 || prec <= min {
			break
		}
		op := p.t
		p.next()
		switch op.kind {
		case "#", "@":
			if p.t.kind != "variable" || p.t.val == "$" || p.t.val == "$$" {
				p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: "S0214", Position: op.pos, Token: op.val, Cause: fmt.Errorf("binding requires variable")}
				break
			}
			if op.kind == "@" {
				stage := n
				if stage.op == "." && len(stage.children) > 1 {
					stage = stage.children[1]
				}
				code := ""
				if stage.op == "filter" {
					code = "S0215"
				}
				if stage.op == "sort" {
					code = "S0216"
				}
				if code != "" {
					p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: code, Position: op.pos, Token: op.val, Cause: fmt.Errorf("invalid focus binding")}
					return n
				}
			}
			name := strings.TrimPrefix(p.t.val, "$")
			p.next()
			n = &node{op: op.kind, val: name, pos: op.pos, children: []*node{n}}
		case "{":
			if n.op == "group" {
				p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: "S0210", Position: op.pos, Token: op.val, Cause: fmt.Errorf("grouping must be final")}
				return n
			}
			group := &node{op: "group", pos: op.pos, children: []*node{n}}
			for p.t.kind != "}" && p.err == nil {
				key := p.parse(0)
				p.expect(":")
				value := p.parse(0)
				group.children = append(group.children, key, value)
				if p.t.kind != "," {
					break
				}
				p.next()
			}
			p.expect("}")
			n = group
		case "^":
			p.expect("(")
			sortNode := &node{op: "sort", pos: op.pos, children: []*node{n}}
			for p.t.kind != ")" && p.err == nil {
				dir := "asc"
				if p.t.kind == "<" || p.t.kind == ">" {
					if p.t.kind == ">" {
						dir = "desc"
					}
					p.next()
				}
				key := p.parse(0)
				sortNode.children = append(sortNode.children, &node{op: dir, children: []*node{key}})
				if p.t.kind != "," {
					break
				}
				p.next()
			}
			p.expect(")")
			n = sortNode
		case "[":
			if n.op == "group" {
				p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: "S0209", Position: op.pos, Token: op.val, Cause: fmt.Errorf("filter after grouping")}
				return n
			}
			if p.t.kind == "]" {
				p.next()
				n = &node{op: "keep", pos: op.pos, children: []*node{n}}
			} else {
				n = &node{op: "filter", pos: op.pos, children: []*node{n, p.parse(0)}}
				p.expect("]")
			}
		case "(":
			n = &node{op: "call", pos: op.pos, children: append([]*node{n}, p.list(")")...)}
		case ":=":
			if n.op != "variable" {
				p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: "S0212", Position: op.pos, Token: op.val, Cause: fmt.Errorf("assignment requires variable")}
				return n
			}
			n = &node{op: op.val, pos: op.pos, children: []*node{n, p.parse(prec - 1)}}
		case ".":
			if n.op == "string" {
				n.op = "name"
			}
			right := p.parse(prec)
			if right != nil && right.op == "number" {
				code := "S0213"
				if p.t.kind == "name" {
					code = "S0201"
				}
				p.err = &jsontrans.Error{Processor: "jsonata", Phase: "compile", Code: code, Position: right.pos, Cause: fmt.Errorf("numeric path step")}
				return n
			}
			for x := right; x != nil; {
				if x.op == "string" {
					x.op = "name"
					break
				}
				if (x.op == "filter" || x.op == "keep") && len(x.children) > 0 {
					x = x.children[0]
				} else {
					break
				}
			}
			n = &node{op: ".", pos: op.pos, children: []*node{n, right}}
		case "?":
			yes := p.parse(0)
			no := &node{op: "undefined"}
			if p.t.kind == ":" {
				p.next()
				no = p.parse(prec - 1)
			}
			n = &node{op: "conditional", pos: op.pos, children: []*node{n, yes, no}}
		default:
			n = &node{op: op.val, pos: op.pos, children: []*node{n, p.parse(prec)}}
		}
	}
	return n
}
func (p *parser) expect(k string) {
	if p.t.kind != k {
		code := "S0202"
		if p.t.kind == "eof" {
			code = "S0203"
		} else if p.t.kind == "!" {
			code = "S0204"
		}
		p.fail(code)
		return
	}
	p.next()
}
func (p *parser) list(end string) []*node {
	var out []*node
	for p.t.kind != end && p.err == nil {
		if err := p.b.Collection(len(out) + 1); err != nil {
			p.err = err
			break
		}
		out = append(out, p.parse(0))
		if p.t.kind != "," {
			break
		}
		p.next()
	}
	p.expect(end)
	return out
}

type evaluator struct {
	trackParents bool
	walkErr      error
	depth        *depthTracker
	objectOrder  map[uintptr]objectOrderEntry
	orderSources map[uintptr]map[uintptr]objectOrderEntry
	ancestor     *ancestor
	b            *budget.Budget
	root         Value
	vars         map[string]Value
	env          environment
	now          time.Time
}

func seq(v Value) []Value {
	if IsUndefined(v) {
		return nil
	}
	switch x := v.(type) {
	case kept:
		return seq(x.value)
	case Sequence:
		return x
	case []any:
		return x
	default:
		return []Value{v}
	}
}
func (e *evaluator) pathItems(v Value) []Value {
	var out []Value
	var walk func(Value)
	walk = func(x Value) {
		if e.walkErr != nil {
			return
		}
		if err := e.b.Enter(); err != nil {
			e.walkErr = err
			return
		}
		defer e.b.Leave()
		appendItem := func(v Value) {
			if err := e.b.Collection(len(out) + 1); err != nil {
				e.walkErr = err
				return
			}
			out = append(out, v)
		}
		switch y := x.(type) {
		case kept:
			walk(y.value)
		case tuple:
			switch nested := y.value.(type) {
			case []any:
				for _, v := range nested {
					walk(tuple{value: v, vars: y.vars, ancestor: y.ancestor})
					if e.walkErr != nil {
						return
					}
				}
			case Sequence:
				for _, v := range nested {
					walk(tuple{value: v, vars: y.vars, ancestor: y.ancestor})
					if e.walkErr != nil {
						return
					}
				}
			default:
				appendItem(y)
			}
		case Sequence:
			for _, v := range y {
				walk(v)
				if e.walkErr != nil {
					return
				}
			}
		case []any:
			for _, v := range y {
				walk(v)
				if e.walkErr != nil {
					return
				}
			}
		default:
			if !IsUndefined(x) {
				appendItem(x)
			}
		}
	}
	walk(v)
	return out
}
func collapse(v Sequence) Value {
	if len(v) == 0 {
		return Undefined
	}
	if len(v) == 1 {
		return v[0]
	}
	return v
}
func (e *evaluator) truth(v Value) bool {
	if e.walkErr != nil {
		return false
	}
	if err := e.b.Enter(); err != nil {
		e.walkErr = err
		return false
	}
	defer e.b.Leave()
	if IsUndefined(v) || v == nil {
		return false
	}
	switch x := v.(type) {
	case tuple:
		return e.truth(x.value)
	case kept:
		return e.truth(x.value)
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case []any:
		for _, v := range x {
			if e.truth(v) {
				return true
			}
			if e.walkErr != nil {
				return false
			}
		}
	case Sequence:
		for _, v := range x {
			if e.truth(v) {
				return true
			}
			if e.walkErr != nil {
				return false
			}
		}
	case map[string]any:
		return len(x) > 0
	}
	return false
}
func (e *evaluator) eval(n *node, focus Value) (result Value, err error) {
	defer func() {
		if e.walkErr != nil {
			result = nil
			err = e.walkErr
		}
	}()
	if n == nil {
		return Undefined, nil
	}
	if err := e.depth.enter(); err != nil {
		return nil, err
	}
	defer e.depth.leave()
	if err := e.b.Enter(); err != nil {
		return nil, err
	}
	defer e.b.Leave()
	switch n.op {
	case "block":
		old := e.vars
		e.vars = copyVars(old)
		defer func() { e.vars = old }()
		var result Value = Undefined
		for _, c := range n.children {
			var err error
			result, err = e.eval(c, focus)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case "transform":
		return &transform{expr: n, eval: e, vars: copyVars(e.vars)}, nil
	case "lambda":
		return &closure{body: n.children[len(n.children)-1], params: n.children[:len(n.children)-1], vars: e.vars, eval: e, focus: focus, sig: n.sig}, nil
	case ":=":
		if n.children[0].op != "variable" {
			return nil, fmt.Errorf("assignment requires variable")
		}
		v, err := e.eval(n.children[1], focus)
		if err != nil {
			return nil, err
		}
		e.vars[strings.TrimPrefix(n.children[0].val.(string), "$")] = v

		return v, nil
	case "parent":
		if e.ancestor == nil {
			return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "S0217", Position: n.pos, Cause: fmt.Errorf("parent unavailable")}
		}
		return tuple{value: e.ancestor.value, ancestor: e.ancestor.up}, nil
	case "undefined":
		return Undefined, nil
	case "placeholder":
		return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "S0211", Position: n.pos, Cause: fmt.Errorf("placeholder outside function arguments")}
	case "number", "string", "literal", "regex":
		return n.val, nil
	case "variable":
		s := n.val.(string)
		if s == "$" {
			return focus, nil
		}
		if s == "$$" {
			return e.root, nil
		}
		if v, ok := e.env.lookup(e.vars, strings.TrimPrefix(s, "$")); ok {
			return v, nil
		}
		return Undefined, nil
	case "name":
		var out Sequence
		contexts := e.pathItems(focus)
		for _, x := range contexts {
			value, _ := unwrapTuple(x)
			parent := e.ancestor
			if t, ok := x.(tuple); ok {
				parent = t.ancestor
			}
			if m, ok := value.(map[string]any); ok {
				if v, exists := m[n.val.(string)]; exists {
					if values, ok := v.([]any); ok && len(contexts) > 1 {
						if err := e.b.Collection(len(out) + len(values)); err != nil {
							return nil, err
						}
						if e.trackParents {
							for _, item := range values {
								out = append(out, tuple{value: item, ancestor: &ancestor{value: value, up: parent}})
							}
						} else {
							out = append(out, values...)
						}
					} else {
						if err := e.b.Collection(len(out) + 1); err != nil {
							return nil, err
						}
						if e.trackParents {
							out = append(out, tuple{value: v, ancestor: &ancestor{value: value, up: parent}})
						} else {
							out = append(out, v)
						}
					}
				}
			}
		}
		return collapse(out), nil
	case "wild*", "wild**":
		var out Sequence
		var walk func(Value) error
		walk = func(v Value) error {
			if err := e.b.Enter(); err != nil {
				return err
			}
			defer e.b.Leave()
			if n.op == "wild**" && !IsUndefined(v) {
				if _, array := v.([]any); !array {
					if err := e.b.Collection(len(out) + 1); err != nil {
						return err
					}
					out = append(out, v)
				}
			}
			switch x := v.(type) {
			case map[string]any:
				keys := make([]string, 0, len(x))
				for k := range x {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if n.op == "wild**" {
						if err := walk(x[k]); err != nil {
							return err
						}
					} else {
						values := e.pathItems(x[k])
						if e.walkErr != nil {
							return e.walkErr
						}
						if err := e.b.Collection(len(out) + len(values)); err != nil {
							return err
						}
						out = append(out, values...)
					}
				}
			case []any:
				for _, v := range x {
					if n.op == "wild**" {
						if err := walk(v); err != nil {
							return err
						}
					} else {
						values := e.pathItems(v)
						if e.walkErr != nil {
							return e.walkErr
						}
						if err := e.b.Collection(len(out) + len(values)); err != nil {
							return err
						}
						out = append(out, values...)
					}
				}
			}
			return e.b.Collection(len(out))
		}
		if err := walk(focus); err != nil {
			return nil, err
		}
		return collapse(out), nil
	case "array":
		out := []any{}
		for _, c := range n.children {
			v, err := e.eval(c, focus)
			if err != nil {
				return nil, err
			}
			if !IsUndefined(v) {
				if s, ok := v.(Sequence); ok {
					if err := e.b.Collection(len(out) + len(s)); err != nil {
						return nil, err
					}
					out = append(out, s...)
				} else if array, ok := v.([]any); ok && c.op != "array" {
					if err := e.b.Collection(len(out) + len(array)); err != nil {
						return nil, err
					}
					out = append(out, array...)
				} else {
					if err := e.b.Collection(len(out) + 1); err != nil {
						return nil, err
					}
					out = append(out, v)
				}
			}
		}
		return out, e.b.Collection(len(out))
	case "object":
		out := map[string]any{}
		order := []string{}
		seen := map[string]bool{}
		for i := 0; i < len(n.children); i += 2 {
			k, err := e.eval(n.children[i], focus)
			if err != nil {
				return nil, err
			}
			k = e.stripTuples(k)
			s, ok := k.(string)
			if !ok {
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T1003", Position: n.pos, Cause: fmt.Errorf("object key must be string")}
			}
			s = jsonvalue.CanonicalString(s)
			if seen[s] {
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "D1009", Position: n.pos, Cause: fmt.Errorf("duplicate object key")}
			}
			if err := e.b.Collection(len(seen) + 1); err != nil {
				return nil, err
			}
			seen[s] = true
			v, err := e.eval(n.children[i+1], focus)
			if err != nil {
				return nil, err
			}
			if !IsUndefined(v) {
				if _, exists := out[s]; !exists {
					if err := e.b.Collection(len(out) + 1); err != nil {
						return nil, err
					}
					order = append(order, s)
				}
				out[s] = v
			}
		}
		e.objectOrder[uintptr(reflect.ValueOf(out).UnsafePointer())] = objectOrderEntry{value: out, keys: order}
		return out, e.b.Collection(len(out))
	case "conditional":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		if e.truth(v) {
			return e.eval(n.children[1], focus)
		}
		return e.eval(n.children[2], focus)
	case "neg":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		if IsUndefined(v) {
			return Undefined, nil
		}
		if f, ok := v.(float64); ok {
			return -f, nil
		}
		return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "D1002", Position: n.pos, Token: "-", Cause: fmt.Errorf("numeric operand required")}
	case "@":
		stage := n.children[0]
		parents := []Value{focus}
		selection := stage
		if stage.op == "." {
			prefix, err := e.eval(stage.children[0], focus)
			if err != nil {
				return nil, err
			}
			parents = e.pathItems(prefix)
			selection = stage.children[1]
		}
		var out Sequence
		for _, parent := range parents {
			value, vars := unwrapTuple(parent)
			selected, err := e.evalWithVars(selection, value, vars)
			if err != nil {
				return nil, err
			}
			selectedItems := e.pathItems(selected)
			if selection.op == "variable" && selection.val == "$" {
				selectedItems = []Value{selected}
			}
			for index, item := range selectedItems {
				itemValue, itemVars := unwrapTuple(item)
				bindings := copyVars(vars)
				for k, v := range itemVars {
					bindings[k] = v
				}
				bindings[n.val.(string)] = itemValue
				var a *ancestor
				if t, ok := parent.(tuple); ok {
					a = t.ancestor
				}
				if e.trackParents && (a == nil || !sameContainer(a.value, value)) {
					a = &ancestor{value: value, up: a}
				}
				if err := e.b.Collection(len(out) + 1); err != nil {
					return nil, err
				}
				out = append(out, tuple{value: value, vars: bindings, ancestor: a, index: index, indexed: true})
			}
		}
		return collapse(out), nil
	case "#":
		stage := n.children[0]
		if stage.op == "." {
			prefix, err := e.eval(stage.children[0], focus)
			if err != nil {
				return nil, err
			}
			var out Sequence
			for _, parent := range e.pathItems(prefix) {
				value, vars := unwrapTuple(parent)
				selected, err := e.evalWithVars(stage.children[1], value, vars)
				if err != nil {
					return nil, err
				}
				for index, item := range e.pathItems(selected) {
					itemValue, itemVars := unwrapTuple(item)
					bindings := copyVars(vars)
					for k, v := range itemVars {
						bindings[k] = v
					}
					bindings[n.val.(string)] = float64(index)
					if err := e.b.Collection(len(out) + 1); err != nil {
						return nil, err
					}
					var ancestry *ancestor
					if t, ok := item.(tuple); ok {
						ancestry = t.ancestor
					} else if e.trackParents {
						var up *ancestor
						if t, ok := parent.(tuple); ok {
							up = t.ancestor
						}
						ancestry = &ancestor{value: value, up: up}
					}
					out = append(out, tuple{value: itemValue, vars: bindings, ancestor: ancestry})
				}
			}
			return collapse(out), nil
		}
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		items := e.pathItems(v)
		out := make(Sequence, 0, len(items))
		for i, item := range items {
			value, vars := unwrapTuple(item)
			vars = copyVars(vars)
			if n.op == "#" {
				index := i
				if t, ok := item.(tuple); ok && n.children[0].op == "@" && t.indexed {
					index = t.index
				}
				vars[n.val.(string)] = float64(index)
			} else {
				vars[n.val.(string)] = value
			}
			var ancestry *ancestor
			if t, ok := item.(tuple); ok {
				ancestry = t.ancestor
			}
			out = append(out, tuple{value: value, vars: vars, ancestor: ancestry})
		}
		return collapse(out), nil
	case "group":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		type groupEntry struct {
			items Sequence
			value *node
			index int
		}
		groups := map[string]*groupEntry{}
		for _, item := range e.pathItems(v) {
			for i := 1; i < len(n.children); i += 2 {
				value, vars := unwrapTuple(item)
				k, err := e.evalWithVars(n.children[i], value, vars)
				if err != nil {
					return nil, err
				}
				if IsUndefined(k) {
					continue
				}
				k = e.stripTuples(k)
				key, ok := k.(string)
				if !ok {
					return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T1003", Position: n.pos, Cause: fmt.Errorf("group key must be string")}
				}
				key = jsonvalue.CanonicalString(key)
				g := groups[key]
				if g == nil {
					if err := e.b.Collection(len(groups) + 1); err != nil {
						return nil, err
					}
					g = &groupEntry{value: n.children[i+1], index: i}
					groups[key] = g
				} else if g.index != i {
					return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "D1009", Position: n.pos, Cause: fmt.Errorf("duplicate group key")}
				}
				if err := e.b.Collection(len(g.items) + 1); err != nil {
					return nil, err
				}
				g.items = append(g.items, item)
			}
		}
		out := map[string]any{}
		for k, g := range groups {
			var values Sequence
			groupedVars := map[string]Sequence{}
			for _, item := range g.items {
				value, vars := unwrapTuple(item)
				values = append(values, value)
				for k, v := range vars {
					groupedVars[k] = append(groupedVars[k], v)
				}
			}
			bindings := map[string]Value{}
			for k, v := range groupedVars {
				bindings[k] = collapse(v)
			}
			value, err := e.evalWithVars(g.value, collapse(values), bindings)
			if err != nil {
				return nil, err
			}
			if !IsUndefined(value) {
				out[k] = value
			}
		}
		return out, e.b.Collection(len(out))
	case "sort":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		items := e.pathItems(v)
		type sortEntry struct {
			value Value
			keys  []Value
		}
		entries := make([]sortEntry, len(items))
		for i, item := range items {
			entries[i].value = item
			for _, criterion := range n.children[1:] {
				value, vars := unwrapTuple(item)
				previous := e.ancestor
				if t, ok := item.(tuple); ok {
					e.ancestor = t.ancestor
				}
				key, err := e.evalWithVars(criterion.children[0], value, vars)
				e.ancestor = previous
				key = e.stripTuples(key)
				if err != nil {
					return nil, err
				}
				entries[i].keys = append(entries[i].keys, key)
			}
		}
		var sortErr error
		sort.SliceStable(entries, func(i, j int) bool {
			if sortErr != nil {
				return false
			}
			if err := e.b.Step(); err != nil {
				sortErr = err
				return false
			}
			for k, c := range n.children[1:] {
				a, b := entries[i].keys[k], entries[j].keys[k]
				if IsUndefined(a) && IsUndefined(b) {
					continue
				}
				if IsUndefined(a) {
					return false
				}
				if IsUndefined(b) {
					return true
				}
				valid := func(v Value) bool {
					switch v.(type) {
					case float64, string:
						return true
					}
					return false
				}
				if !valid(a) || !valid(b) {
					sortErr = &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2008", Position: n.pos, Cause: fmt.Errorf("invalid sort key")}
					return false
				}
				var less, greater bool
				switch x := a.(type) {
				case float64:
					y, ok := b.(float64)
					if !ok {
						sortErr = &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2007", Position: n.pos, Cause: fmt.Errorf("sort key types differ")}
						return false
					}
					less = x < y
					greater = x > y
				case string:
					y, ok := b.(string)
					if !ok {
						sortErr = &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2007", Position: n.pos, Cause: fmt.Errorf("sort key types differ")}
						return false
					}
					less = x < y
					greater = x > y
				default:
					sortErr = &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2008", Position: n.pos, Cause: fmt.Errorf("invalid sort key")}
					return false
				}
				if less || greater {
					if c.op == "desc" {
						return greater
					}
					return less
				}
			}
			return false
		})
		if sortErr != nil {
			return nil, sortErr
		}
		out := make(Sequence, len(entries))
		for i := range entries {
			out[i] = entries[i].value
		}
		if len(out) == 0 {
			return Undefined, nil
		}
		if _, keep := v.(kept); keep {
			return kept{out}, nil
		}
		return collapse(out), nil
	case "keep":
		v, err := e.eval(n.children[0], focus)
		if err != nil || IsUndefined(v) {
			return v, err
		}
		switch v.(type) {
		case Sequence, []any:
			return kept{v}, nil
		default:
			return kept{Sequence{v}}, nil
		}
	case "filter":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		items := seq(v)
		if e.trackParents {
			items = e.pathItems(v)
		}
		var out Sequence
		for i, item := range items {
			value, vars := unwrapTuple(item)
			previous := e.ancestor
			if e.trackParents {
				if t, ok := item.(tuple); ok {
					e.ancestor = t.ancestor
				} else {
					e.ancestor = &ancestor{value: focus, up: previous}
				}
			}
			r, err := e.evalWithVars(n.children[1], value, vars)
			e.ancestor = previous
			if err != nil {
				return nil, err
			}
			indices := seq(r)
			numeric := len(indices) > 0
			for _, x := range indices {
				if _, ok := x.(float64); !ok {
					numeric = false
					break
				}
			}
			if numeric {
				for _, x := range indices {
					idx := int(math.Floor(x.(float64)))
					if idx < 0 {
						idx += len(items)
					}
					if i == idx {
						out = append(out, item)
						break
					}
				}
			} else if e.truth(r) {
				out = append(out, item)
			}
		}
		if len(out) == 0 {
			return Undefined, nil
		}
		if _, keep := v.(kept); keep {
			return kept{out}, nil
		}
		return collapse(out), nil
	case ".":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		var out Sequence
		_, keepResult := v.(kept)
		items := e.pathItems(v)
		for _, item := range items {
			value, vars := unwrapTuple(item)
			previous := e.ancestor
			if t, ok := item.(tuple); ok {
				e.ancestor = t.ancestor
			}
			r, err := e.evalWithVars(n.children[1], value, vars)
			itemAncestor := e.ancestor
			e.ancestor = previous
			if err != nil {
				return nil, err
			}
			add := func(x Value) {
				if e.walkErr != nil {
					return
				}
				if err := e.b.Collection(len(out) + 1); err != nil {
					e.walkErr = err
					return
				}
				if IsUndefined(x) {
					return
				}
				if vars != nil || e.trackParents {
					if inner, ok := x.(tuple); ok {
						merged := copyVars(vars)
						for k, v := range inner.vars {
							merged[k] = v
						}
						x = tuple{value: inner.value, vars: merged, ancestor: inner.ancestor}
					} else {
						var parent *ancestor
						if e.trackParents {
							parent = &ancestor{value: value, up: itemAncestor}
						}
						x = tuple{value: x, vars: vars, ancestor: parent}
					}
				}
				out = append(out, x)
			}
			if k, ok := r.(kept); ok {
				keepResult = true
				switch x := k.value.(type) {
				case Sequence:
					for _, v := range x {
						add(v)
					}
				default:
					add(x)
				}
				continue
			}
			if values, ok := r.([]any); ok && e.trackParents && n.children[1].op == "array" {
				for _, item := range values {
					add(item)
				}
				continue
			}
			if values, ok := r.([]any); ok && len(values) == 0 && n.children[1].op != "array" {
				continue
			}
			if values, ok := r.([]any); ok && n.children[1].op == "name" && len(items) > 1 {
				for _, x := range values {
					add(x)
				}
				continue
			}
			if values, ok := r.(Sequence); ok {
				for _, x := range values {
					add(x)
				}
			} else {
				add(r)
			}
		}
		if err := e.b.Collection(len(out)); err != nil {
			return nil, err
		}
		if keepResult {
			return kept{out}, nil
		}
		return collapse(out), nil
	case "~>":
		left, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		right := n.children[1]
		keepChain := false
		if right.op == "keep" {
			keepChain = true
			right = right.children[0]
		}
		hasPlaceholder := false
		if right.op == "call" {
			for _, arg := range right.children[1:] {
				if arg.op == "placeholder" {
					hasPlaceholder = true
					break
				}
			}
		}
		if right.op == "call" && !hasPlaceholder {
			f, err := e.eval(right.children[0], focus)
			if err != nil {
				return nil, err
			}
			fn, ok := f.(Callable)
			if !ok {
				return nil, fmt.Errorf("not a function")
			}
			args := []Value{e.stripTuples(left)}
			for _, c := range right.children[1:] {
				v, err := e.eval(c, focus)
				if err != nil {
					return nil, err
				}
				args = append(args, e.stripTuples(v))
			}
			result, err := e.invoke(fn, args, focus)
			if err != nil {
				return nil, callErrorPosition(err, right.pos)
			}
			if keepChain && !IsUndefined(result) {
				switch result.(type) {
				case Sequence, []any:
					return kept{result}, nil
				default:
					return kept{Sequence{result}}, nil
				}
			}
			return result, nil
		}
		v, err := e.eval(right, focus)
		if err != nil {
			return nil, err
		}
		fn, ok := v.(Callable)
		if !ok {
			return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2006", Position: n.pos, Cause: fmt.Errorf("chain requires function")}
		}
		if f, ok := left.(Callable); ok {
			return composed{first: f, second: fn}, nil
		}
		return e.invoke(fn, []Value{left}, focus)
	case "call":
		if n.children[0].op == "parent" && e.ancestor == nil {
			return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T1006", Position: n.pos, Cause: fmt.Errorf("not a function")}
		}
		f, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		fn, ok := f.(Callable)
		if !ok {
			code := "T1006"
			if n.children[0].op == "name" {
				if _, exists := e.env.lookup(e.vars, n.children[0].val.(string)); exists {
					code = "T1005"
				}
			}
			for _, arg := range n.children[1:] {
				if arg.op == "placeholder" {
					code = "T1008"
					if n.children[0].op == "name" {
						if _, exists := e.env.lookup(e.vars, n.children[0].val.(string)); exists {
							code = "T1007"
						}
					}
					break
				}
			}
			return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: code, Position: n.pos, Cause: fmt.Errorf("not a function")}
		}
		args := make([]Value, 0, len(n.children)-1)
		hasPlaceholder := false
		for _, c := range n.children[1:] {
			if c.op == "placeholder" {
				args = append(args, placeholder{})
				hasPlaceholder = true
				continue
			}
			v, err := e.eval(c, focus)
			if err != nil {
				return nil, err
			}
			args = append(args, e.stripTuples(v))
		}
		if hasPlaceholder {
			return partial{fn: fn, args: args}, nil
		}
		result, err := e.invoke(fn, args, focus)
		return result, callErrorPosition(err, n.pos)
	}
	a, err := e.eval(n.children[0], focus)
	if err != nil {
		return nil, err
	}
	if n.op == "and" && !e.truth(a) {
		return false, nil
	}
	if n.op == "or" && e.truth(a) {
		return true, nil
	}
	if n.op == "??" && !IsUndefined(a) {
		return a, nil
	}
	if n.op == "?:" && e.truth(a) {
		return a, nil
	}
	b, err := e.eval(n.children[1], focus)
	if err != nil {
		return nil, err
	}
	a = e.stripTuples(a)
	b = e.stripTuples(b)
	switch n.op {
	case "in":
		if IsUndefined(a) || IsUndefined(b) {
			return false, nil
		}
		for _, v := range seq(b) {
			if reflect.DeepEqual(a, v) {
				return true, nil
			}
		}
		return false, nil
	case "and":
		return e.truth(b), nil
	case "or":
		return e.truth(b), nil
	case "??", "?:":
		return b, nil
	case "=":
		if IsUndefined(a) || IsUndefined(b) {
			return false, nil
		}
		left, err := materializeChecked(a, e.b)
		if err != nil {
			return nil, err
		}
		right, err := materializeChecked(b, e.b)
		if err != nil {
			return nil, err
		}
		return reflect.DeepEqual(left, right), nil
	case "!=":
		if IsUndefined(a) || IsUndefined(b) {
			return false, nil
		}
		left, err := materializeChecked(a, e.b)
		if err != nil {
			return nil, err
		}
		right, err := materializeChecked(b, e.b)
		if err != nil {
			return nil, err
		}
		return !reflect.DeepEqual(left, right), nil
	case "&":
		left, err := stringValue(a, e.b)
		if err != nil {
			return nil, err
		}
		right, err := stringValue(b, e.b)
		if err != nil {
			return nil, err
		}
		if len(left) > e.b.Limits.MaxOutputBytes-len(right) {
			return nil, budget.ErrLimit
		}
		return jsonvalue.CanonicalString(left + right), nil
	case "<", ">", "<=", ">=":
		if x, ok := a.(string); ok {
			if y, ok := b.(string); ok {
				switch n.op {
				case "<":
					return x < y, nil
				case ">":
					return x > y, nil
				case "<=":
					return x <= y, nil
				case ">=":
					return x >= y, nil
				}
			}
		}
	}
	if n.op == ".." {
		validate := func(v Value, code string) error {
			if IsUndefined(v) {
				return nil
			}
			x, ok := v.(float64)
			if !ok || x != math.Trunc(x) {
				return &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: code, Position: n.pos, Token: "..", Cause: fmt.Errorf("range endpoint must be an integer")}
			}
			return nil
		}
		if err := validate(a, "T2003"); err != nil {
			return nil, err
		}
		if err := validate(b, "T2004"); err != nil {
			return nil, err
		}
		if !IsUndefined(a) && !IsUndefined(b) {
			if b.(float64)-a.(float64)+1 > 1e7 {
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "D2014", Position: n.pos, Token: "..", Cause: fmt.Errorf("range too large")}
			}
		}
	}
	if n.op == "+" || n.op == "-" || n.op == "*" || n.op == "/" || n.op == "%" {
		for i, v := range []Value{a, b} {
			if IsUndefined(v) {
				continue
			}
			if number, ok := v.(float64); ok && (math.IsInf(number, 0) || math.IsNaN(number)) {
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "D1001", Position: n.pos, Token: n.op, Cause: fmt.Errorf("nonfinite number")}
			}
			if _, ok := v.(float64); !ok {
				code := "T2001"
				if i == 1 {
					code = "T2002"
				}
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: code, Position: n.pos, Token: n.op, Cause: fmt.Errorf("numeric operand required")}
			}
		}
	}
	if n.op == "<" || n.op == ">" || n.op == "<=" || n.op == ">=" {
		for _, v := range []Value{a, b} {
			if IsUndefined(v) {
				continue
			}
			switch v.(type) {
			case float64, string:
			default:
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2010", Position: n.pos, Token: n.op, Cause: fmt.Errorf("invalid comparison")}
			}
		}
	}
	if IsUndefined(a) || IsUndefined(b) {
		return Undefined, nil
	}
	x, xok := a.(float64)
	y, yok := b.(float64)
	if !xok || !yok {
		code := "T2001"
		if xok {
			code = "T2002"
		}
		if n.op == "<" || n.op == ">" || n.op == "<=" || n.op == ">=" {
			code = "T2010"
			valid := func(v Value) bool {
				switch v.(type) {
				case float64, string:
					return true
				}
				return false
			}
			if valid(a) && valid(b) {
				code = "T2009"
			}
		}
		return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: code, Position: n.pos, Token: n.op, Cause: fmt.Errorf("numeric operands required")}
	}
	switch n.op {
	case "..":
		if x != math.Trunc(x) || y != math.Trunc(y) {
			return nil, fmt.Errorf("range endpoints must be integers")
		}
		if y < x {
			return Undefined, nil
		}
		if y-x+1 > float64(e.b.Limits.MaxCollectionItems) {
			return nil, budget.ErrLimit
		}
		out := make(Sequence, 0, int(y-x)+1)
		for v := x; v <= y; v++ {
			if err := e.b.Step(); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case "+":
		return finiteNumber(x+y, n)
	case "-":
		return finiteNumber(x-y, n)
	case "*":
		return finiteNumber(x*y, n)
	case "/":
		return finiteNumber(x/y, n)
	case "%":
		return finiteNumber(math.Mod(x, y), n)
	case "<":
		return x < y, nil
	case ">":
		return x > y, nil
	case "<=":
		return x <= y, nil
	case ">=":
		return x >= y, nil
	}
	return nil, fmt.Errorf("unsupported operator %s", n.op)
}

func copyVars(src map[string]Value) map[string]Value {
	dst := make(map[string]Value, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

type closure struct {
	body   *node
	params []*node
	vars   map[string]Value
	eval   *evaluator
	focus  Value
	sig    *functionSignature
}

func (c *closure) Arity() int { return len(c.params) }
func (c *closure) Call(ctx *CallContext, args []Value) (Value, error) {
	current := c
	for {
		if err := ctx.Budget.Step(); err != nil {
			return nil, err
		}
		if current.sig != nil {
			var err error
			args, err = current.sig.validate(args, ctx.Focus)
			if err != nil {
				return nil, err
			}
		}
		vars := copyVars(current.vars)
		for i, p := range current.params {
			if p.op != "variable" {
				return nil, fmt.Errorf("invalid parameter")
			}
			v := Undefined
			if i < len(args) {
				v = args[i]
			}
			vars[strings.TrimPrefix(p.val.(string), "$")] = v
		}
		ev := *current.eval
		ev.b = ctx.Budget
		ev.now = ctx.Now
		if ctx.state != nil {
			if ctx.state.orderSources == nil {
				ctx.state.orderSources = map[uintptr]map[uintptr]objectOrderEntry{}
			}
			merged, err := mergeInvocationOrder(ctx.state.objectOrder, current.eval.objectOrder, ctx.state.orderSources, current.eval.b.Context(), ctx.Budget)
			if err != nil {
				return nil, err
			}
			ctx.state.objectOrder = merged
			ev.objectOrder = merged
			ev.orderSources = ctx.state.orderSources
			ev.depth = ctx.state.depth
		}
		ev.vars = vars
		result, err := ev.evalTail(current.body, current.focus)
		if err != nil {
			return nil, err
		}
		if next, ok := result.(tailCall); ok {
			current = next.fn
			args = next.args
			continue
		}
		return result, nil
	}
}

type tailCall struct {
	fn   *closure
	args []Value
}

func (e *evaluator) evalTail(n *node, focus Value) (result Value, err error) {
	defer func() {
		if e.walkErr != nil {
			result = nil
			err = e.walkErr
		}
	}()
	if n == nil {
		return Undefined, nil
	}
	if err := e.b.Step(); err != nil {
		return nil, err
	}
	entered := false
	if n.op == "conditional" || n.op == "block" || n.op == "call" {
		if err := e.depth.enter(); err != nil {
			return nil, err
		}
		entered = true
	}
	defer func() {
		if entered {
			e.depth.leave()
		}
	}()
	switch n.op {
	case "conditional":
		v, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		if e.truth(v) {
			return e.evalTail(n.children[1], focus)
		}
		return e.evalTail(n.children[2], focus)
	case "block":
		old := e.vars
		e.vars = copyVars(old)
		defer func() { e.vars = old }()
		if len(n.children) == 0 {
			return Undefined, nil
		}
		for _, child := range n.children[:len(n.children)-1] {
			if _, err := e.eval(child, focus); err != nil {
				return nil, err
			}
		}
		return e.evalTail(n.children[len(n.children)-1], focus)
	case "call":
		if n.children[0].op == "parent" && e.ancestor == nil {
			return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T1006", Position: n.pos, Cause: fmt.Errorf("not a function")}
		}
		f, err := e.eval(n.children[0], focus)
		if err != nil {
			return nil, err
		}
		fn, ok := f.(*closure)
		if !ok {
			if entered {
				e.depth.leave()
				entered = false
			}
			return e.eval(n, focus)
		}
		args := make([]Value, 0, len(n.children)-1)
		for _, child := range n.children[1:] {
			if child.op == "placeholder" {
				if entered {
					e.depth.leave()
					entered = false
				}
				return e.eval(n, focus)
			}
			v, err := e.eval(child, focus)
			if err != nil {
				return nil, err
			}
			args = append(args, e.stripTuples(v))
		}
		return tailCall{fn: fn, args: args}, nil
	}
	return e.eval(n, focus)
}

func stringValue(v Value, b *budget.Budget) (string, error) {
	if IsUndefined(v) {
		return "", nil
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return strconv.FormatFloat(x, 'g', 15, 64), nil
	case nil:
		return "null", nil
	case bool:
		return strconv.FormatBool(x), nil
	default:
		normalized, err := materializeChecked(v, b)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(normalized)
		if err != nil {
			return "", err
		}
		if len(encoded) > b.Limits.MaxOutputBytes {
			return "", budget.ErrLimit
		}
		return string(encoded), nil
	}
}

type ancestor struct {
	value Value
	up    *ancestor
}
type tuple struct {
	value    Value
	vars     map[string]Value
	ancestor *ancestor
	index    int
	indexed  bool
}

func unwrapTuple(v Value) (Value, map[string]Value) {
	if t, ok := v.(tuple); ok {
		return t.value, t.vars
	}
	return v, nil
}
func (e *evaluator) evalWithVars(n *node, focus Value, vars map[string]Value) (Value, error) {
	if vars == nil {
		return e.eval(n, focus)
	}
	old := e.vars
	e.vars = copyVars(old)
	for k, v := range vars {
		e.vars[k] = v
	}
	defer func() { e.vars = old }()
	return e.eval(n, focus)
}

type kept struct{ value Value }

func (e *evaluator) invoke(fn Callable, args []Value, focus Value) (Value, error) {
	cc := &CallContext{Context: e.b.Context(), Input: e.root, Focus: focus, Budget: e.b, Now: e.now, state: e}
	cc.ObjectKeys = func(v Value) []string {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		entry, ok := e.objectOrder[uintptr(reflect.ValueOf(m).UnsafePointer())]
		if ok {
			return jsonvalue.EnumerationKeys(entry.keys)
		}
		return jsonvalue.ObjectKeys(e.b.Context(), m)
	}
	cc.Invoke = func(f Callable, a []Value) (Value, error) {
		v, err := f.Call(cc, a)
		if err != nil {
			return nil, err
		}
		if text, ok := v.(string); ok {
			v = jsonvalue.CanonicalString(text)
		}
		return v, e.b.Check()
	}
	cc.Evaluate = func(expr string, input Value) (Value, error) {
		if err := e.b.Expression(expr); err != nil {
			return nil, err
		}
		p := parser{s: expr, b: e.b}
		p.next()
		root := p.parse(0)
		if p.err == nil && p.t.kind != "eof" {
			p.fail("S0201")
		}
		if p.err != nil {
			return nil, convertCompilePosition(p.err, expr)
		}
		if err := convertNodePositions(root, expr, e.b); err != nil {
			return nil, err
		}
		nested := *e
		nested.trackParents = containsParent(root)
		nested.vars = copyVars(e.vars)
		return nested.eval(root, input)
	}
	return cc.Invoke(fn, args)
}

type composed struct{ first, second Callable }

func (c composed) Arity() int { return c.first.Arity() }
func (c composed) Call(ctx *CallContext, args []Value) (Value, error) {
	v, err := ctx.Invoke(c.first, args)
	if err != nil {
		return nil, err
	}
	return ctx.Invoke(c.second, []Value{v})
}

type transform struct {
	expr *node
	eval *evaluator
	vars map[string]Value
}

func (t *transform) Arity() int { return 1 }
func (t *transform) Call(ctx *CallContext, args []Value) (Value, error) {
	if len(args) == 0 || IsUndefined(args[0]) {
		return Undefined, nil
	}
	value, err := materializeChecked(args[0], ctx.Budget)
	if err != nil {
		return nil, err
	}
	copied, err := jsonvalue.Clone(value, ctx.Budget, ctx.Budget.Limits.MaxInputBytes)
	if err != nil {
		return nil, err
	}
	copied = normalize(copied)
	ev := *t.eval
	ev.b = ctx.Budget
	ev.now = ctx.Now
	if ctx.state != nil {
		if ctx.state.orderSources == nil {
			ctx.state.orderSources = map[uintptr]map[uintptr]objectOrderEntry{}
		}
		merged, err := mergeInvocationOrder(ctx.state.objectOrder, t.eval.objectOrder, ctx.state.orderSources, t.eval.b.Context(), ctx.Budget)
		if err != nil {
			return nil, err
		}
		ctx.state.objectOrder = merged
		ev.objectOrder = merged
		ev.orderSources = ctx.state.orderSources
		ev.depth = ctx.state.depth
	}
	ev.vars = copyVars(t.vars)
	match, err := ev.eval(t.expr.children[0], copied)
	if err != nil {
		return nil, err
	}
	items := ev.pathItems(match)
	if ev.walkErr != nil {
		return nil, ev.walkErr
	}
	for _, item := range items {
		if err := ctx.Budget.Step(); err != nil {
			return nil, err
		}
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		update, err := ev.eval(t.expr.children[1], object)
		if err != nil {
			return nil, err
		}
		if !IsUndefined(update) {
			changes, ok := update.(map[string]any)
			if !ok {
				return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2011", Position: t.expr.pos, Cause: fmt.Errorf("transform update requires object")}
			}
			for k, v := range changes {
				object[k] = v
			}
		}
		if len(t.expr.children) > 2 {
			del, err := ev.eval(t.expr.children[2], object)
			if err != nil {
				return nil, err
			}
			if !IsUndefined(del) {
				for _, key := range seq(del) {
					s, ok := key.(string)
					if !ok {
						return nil, &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "T2012", Position: t.expr.pos, Cause: fmt.Errorf("transform delete requires string")}
					}
					delete(object, s)
				}
			}
		}
	}
	return copied, nil
}

type placeholder struct{}
type partial struct {
	fn   Callable
	args []Value
}

func (p partial) Arity() int {
	n := 0
	for _, v := range p.args {
		if _, ok := v.(placeholder); ok {
			n++
		}
	}
	return n
}
func (p partial) Call(ctx *CallContext, args []Value) (Value, error) {
	values := make([]Value, len(p.args))
	i := 0
	for j, v := range p.args {
		if _, ok := v.(placeholder); ok {
			values[j] = Undefined
			if i < len(args) {
				values[j] = args[i]
			}
			i++
		} else {
			values[j] = v
		}
	}
	if i < len(args) {
		values = append(values, args[i:]...)
	}
	return ctx.Invoke(p.fn, values)
}

func finiteNumber(v float64, n *node) (Value, error) { return v, nil }

// materializeChecked bounds output conversion and detects cycles returned by host functions.
func materializeChecked(v Value, b *budget.Budget) (Value, error) {
	active := map[uintptr]bool{}
	var convert func(Value) (Value, error)
	convert = func(v Value) (Value, error) {
		if err := b.Enter(); err != nil {
			return nil, err
		}
		defer b.Leave()
		if IsUndefined(v) {
			return nil, nil
		}
		switch x := v.(type) {
		case tuple:
			return convert(x.value)
		case kept:
			return convert(x.value)
		case Sequence:
			if err := b.Collection(len(x)); err != nil {
				return nil, err
			}
			ptr := reflect.ValueOf(x).Pointer()
			if ptr != 0 && active[ptr] {
				return nil, fmt.Errorf("%w: cycle", jsontrans.ErrInvalidInput)
			}
			if ptr != 0 {
				active[ptr] = true
				defer delete(active, ptr)
			}
			out := make([]any, len(x))
			for i, item := range x {
				y, err := convert(item)
				if err != nil {
					return nil, err
				}
				out[i] = y
			}
			return out, nil
		case []any:
			if err := b.Collection(len(x)); err != nil {
				return nil, err
			}
			ptr := reflect.ValueOf(x).Pointer()
			if ptr != 0 && active[ptr] {
				return nil, fmt.Errorf("%w: cycle", jsontrans.ErrInvalidInput)
			}
			if ptr != 0 {
				active[ptr] = true
				defer delete(active, ptr)
			}
			out := make([]any, len(x))
			for i, item := range x {
				y, err := convert(item)
				if err != nil {
					return nil, err
				}
				out[i] = y
			}
			return out, nil
		case map[string]any:
			if err := b.Collection(len(x)); err != nil {
				return nil, err
			}
			ptr := uintptr(reflect.ValueOf(x).UnsafePointer())
			if ptr != 0 && active[ptr] {
				return nil, fmt.Errorf("%w: cycle", jsontrans.ErrInvalidInput)
			}
			if ptr != 0 {
				active[ptr] = true
				defer delete(active, ptr)
			}
			out := make(map[string]any, len(x))
			for k, item := range x {
				if IsUndefined(item) {
					continue
				}
				y, err := convert(item)
				if err != nil {
					return nil, err
				}
				out[k] = y
			}
			return out, nil
		default:
			return v, nil
		}
	}
	return convert(v)
}

func containsParent(n *node) bool {
	if n == nil {
		return false
	}
	if n.op == "parent" {
		return true
	}
	for _, c := range n.children {
		if containsParent(c) {
			return true
		}
	}
	return false
}

type objectOrderEntry struct {
	value map[string]any
	keys  []string
}

// Invoke calls a raw JSONata function with a fresh, evaluation-local budget.
// JSON-shaped arguments are detached before calling the function.
func Invoke(ctx context.Context, fn Callable, args []Value, opts jsontrans.EvalOptions) (Value, error) {
	if fn == nil {
		return nil, fmt.Errorf("nil function")
	}
	limits, err := budget.Resolve(budget.DefaultLimits(), opts.Limits)
	if err != nil {
		return nil, err
	}
	b, err := budget.New(ctx, limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	values, err := cloneRawArgs(args, b)
	if err != nil {
		return nil, err
	}
	ev := evaluator{b: b, root: Undefined, vars: map[string]Value{}, now: time.Now(), objectOrder: map[uintptr]objectOrderEntry{}}
	v, err := ev.invoke(fn, values, Undefined)
	if err != nil {
		return nil, err
	}
	return v, b.Check()
}
func cloneRawArgs(args []Value, b *budget.Budget) ([]Value, error) {
	remaining := b.Limits.MaxInputBytes
	active := map[uintptr]bool{}
	var clone func(Value) (Value, error)
	clone = func(v Value) (Value, error) {
		if err := b.Enter(); err != nil {
			return nil, err
		}
		defer b.Leave()
		remaining--
		if remaining < 0 {
			return nil, budget.ErrLimit
		}
		if IsUndefined(v) {
			return Undefined, nil
		}
		if _, ok := v.(Callable); ok {
			return v, nil
		}
		if x, ok := v.(jsontrans.UTF16Object); ok {
			v = map[string]any(x)
		}
		switch x := v.(type) {
		case Sequence:
			if err := b.Collection(len(x)); err != nil {
				return nil, err
			}
			ptr := reflect.ValueOf(x).Pointer()
			if ptr != 0 && active[ptr] {
				return nil, fmt.Errorf("%w: cycle", jsontrans.ErrInvalidInput)
			}
			if ptr != 0 {
				active[ptr] = true
				defer delete(active, ptr)
			}
			out := make(Sequence, len(x))
			for i, v := range x {
				y, err := clone(v)
				if err != nil {
					return nil, err
				}
				out[i] = y
			}
			return out, nil
		case []any:
			if x == nil {
				return nil, nil
			}
			if err := b.Collection(len(x)); err != nil {
				return nil, err
			}
			ptr := reflect.ValueOf(x).Pointer()
			if ptr != 0 && active[ptr] {
				return nil, fmt.Errorf("%w: cycle", jsontrans.ErrInvalidInput)
			}
			if ptr != 0 {
				active[ptr] = true
				defer delete(active, ptr)
			}
			out := make([]any, len(x))
			for i, v := range x {
				y, err := clone(v)
				if err != nil {
					return nil, err
				}
				out[i] = y
			}
			return out, nil
		case map[string]any:
			if x == nil {
				return nil, nil
			}
			if err := b.Collection(len(x)); err != nil {
				return nil, err
			}
			ptr := uintptr(reflect.ValueOf(x).UnsafePointer())
			if ptr != 0 && active[ptr] {
				return nil, fmt.Errorf("%w: cycle", jsontrans.ErrInvalidInput)
			}
			if ptr != 0 {
				active[ptr] = true
				defer delete(active, ptr)
			}
			out := make(map[string]any, len(x))
			for k, v := range x {
				if _, err := jsonvalue.Quote(k); err != nil {
					return nil, fmt.Errorf("%w: invalid object key", jsontrans.ErrInvalidInput)
				}
				remaining -= len(k)
				if remaining < 0 {
					return nil, budget.ErrLimit
				}
				y, err := clone(v)
				if err != nil {
					return nil, err
				}
				out[k] = y
			}
			return out, nil
		case string:
			remaining -= len(x)
			if remaining < 0 {
				return nil, budget.ErrLimit
			}
		}
		y, err := jsonvalue.Clone(v, b, b.Limits.MaxInputBytes)
		if err != nil {
			return nil, err
		}
		y = normalize(y)
		if err := validateFinite(y, b); err != nil {
			return nil, err
		}
		return y, nil
	}
	out := make([]Value, len(args))
	if err := b.Collection(len(args)); err != nil {
		return nil, err
	}
	for i, v := range args {
		y, err := clone(v)
		if err != nil {
			return nil, err
		}
		out[i] = y
	}
	return out, nil
}

func escapeLiteralControls(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r < 0x20 {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Limits returns the engine's configured resource limits.
func (e *Engine) Limits() jsontrans.Limits { return e.limits }

// Limits returns the program's configured resource limits.
func (p *Program) Limits() jsontrans.Limits { return p.engine.limits }

func (e *evaluator) stripTuples(v Value) Value {
	if e.walkErr != nil {
		return Undefined
	}
	if err := e.b.Enter(); err != nil {
		e.walkErr = err
		return Undefined
	}
	defer e.b.Leave()
	switch x := v.(type) {
	case tuple:
		return e.stripTuples(x.value)
	case Sequence:
		if err := e.b.Collection(len(x)); err != nil {
			e.walkErr = err
			return Undefined
		}
		out := make(Sequence, len(x))
		for i, v := range x {
			out[i] = e.stripTuples(v)
			if e.walkErr != nil {
				return Undefined
			}
		}
		return out
	case kept:
		return kept{e.stripTuples(x.value)}
	default:
		return v
	}
}

func utf16Position(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	count := 0
	for _, r := range source[:offset] {
		n := utf16.RuneLen(r)
		if n < 1 {
			n = 1
		}
		count += n
	}
	return count
}
func convertNodePositions(root *node, source string, b *budget.Budget) error {
	positions := make([]int, len(source)+1)
	count := 0
	for i := 0; i < len(source); {
		if err := b.Step(); err != nil {
			return err
		}
		r, n := utf8.DecodeRuneInString(source[i:])
		if n == 1 && i+2 < len(source) && source[i] == 0xed && source[i+1] >= 0xa0 && source[i+1] <= 0xbf && source[i+2] >= 0x80 && source[i+2] <= 0xbf {
			n = 3
		}
		for j := 0; j < n; j++ {
			positions[i+j] = count
		}
		width := utf16.RuneLen(r)
		if width < 1 {
			width = 1
		}
		count += width
		i += n
		positions[i] = count
	}
	var walk func(*node) error
	walk = func(n *node) error {
		if n == nil {
			return nil
		}
		if err := b.Enter(); err != nil {
			return err
		}
		defer b.Leave()
		pos := n.pos
		if pos < 0 {
			pos = 0
		}
		if pos > len(source) {
			pos = len(source)
		}
		n.pos = positions[pos]
		for _, child := range n.children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}
func convertCompilePosition(err error, source string) error {
	var native *jsontrans.Error
	if errors.As(err, &native) {
		copy := *native
		copy.Position = utf16Position(source, copy.Position)
		return &copy
	}
	return err
}
func callErrorPosition(err error, pos int) error {
	if err == nil {
		return nil
	}
	var native *jsontrans.Error
	if errors.As(err, &native) && native.Position == 0 {
		copy := *native
		copy.Position = pos
		return &copy
	}
	return err
}

// decodeStringLiteral retains unpaired UTF-16 surrogates as WTF-8 bytes so
// language functions can distinguish them from a replacement character.
func decodeStringLiteral(raw string) (string, error) {
	if len(raw) < 2 {
		return "", fmt.Errorf("unterminated string")
	}
	var b strings.Builder
	for i := 1; i < len(raw)-1; i++ {
		c := raw[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(raw)-1 {
			return "", fmt.Errorf("unterminated escape")
		}
		switch raw[i] {
		case '\'', '"', '\\', '/':
			b.WriteByte(raw[i])
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			if i+4 >= len(raw) {
				return "", fmt.Errorf("invalid unicode escape")
			}
			v, err := strconv.ParseUint(raw[i+1:i+5], 16, 16)
			if err != nil {
				return "", err
			}
			i += 4
			r := rune(v)
			if r >= 0xD800 && r <= 0xDBFF && i+6 < len(raw) && raw[i+1] == '\\' && raw[i+2] == 'u' {
				next, err := strconv.ParseUint(raw[i+3:i+7], 16, 16)
				if err == nil && next >= 0xDC00 && next <= 0xDFFF {
					r = utf16.DecodeRune(r, rune(next))
					i += 6
				}
			}
			if r >= 0xD800 && r <= 0xDFFF {
				b.WriteByte(byte(0xe0 | r>>12))
				b.WriteByte(byte(0x80 | (r>>6)&0x3f))
				b.WriteByte(byte(0x80 | r&0x3f))
			} else {
				b.WriteRune(r)
			}
		default:
			return "", fmt.Errorf("invalid escape")
		}
	}
	return b.String(), nil
}

func (p *Program) wrapTimebox(err error) error {
	if err != nil && p.engine.evaluationDepth > 0 && errors.Is(err, context.DeadlineExceeded) {
		return &jsontrans.Error{Processor: "jsonata", Phase: "evaluate", Code: "U1001", Cause: fmt.Errorf("%w: %w", jsontrans.ErrLimit, err)}
	}
	return err
}

func sameContainer(a, b Value) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		return ok && reflect.ValueOf(x).UnsafePointer() == reflect.ValueOf(y).UnsafePointer()
	case []any:
		y, ok := b.([]any)
		return ok && reflect.ValueOf(x).Pointer() == reflect.ValueOf(y).Pointer()
	}
	return false
}

// mergeInvocationOrder retains captured provenance without writing to a map
// shared by returned functions. Each invocation owns the resulting registry.
func mergeInvocationOrder(local, captured map[uintptr]objectOrderEntry, sources map[uintptr]map[uintptr]objectOrderEntry, source context.Context, b *budget.Budget) (map[uintptr]objectOrderEntry, error) {
	var sourceKey uintptr
	if captured != nil {
		sourceKey = uintptr(reflect.ValueOf(captured).UnsafePointer())
	}
	if sourceKey != 0 && sources[sourceKey] != nil {
		return local, nil
	}
	if local != nil && captured != nil && reflect.ValueOf(local).UnsafePointer() == reflect.ValueOf(captured).UnsafePointer() {
		sources[sourceKey] = captured
		return local, nil
	}
	result := make(map[uintptr]objectOrderEntry)
	add := func(key uintptr, entry objectOrderEntry) error {
		if err := b.Step(); err != nil {
			return err
		}
		if _, ok := result[key]; !ok {
			if err := b.Collection(len(result) + 1); err != nil {
				return err
			}
		}
		result[key] = entry
		return nil
	}
	for key, entry := range captured {
		if err := add(key, entry); err != nil {
			return nil, err
		}
	}
	err := jsonvalue.VisitObjectOrders(source, b, func(entry jsonvalue.ObjectOrderEntry) error {
		key := uintptr(reflect.ValueOf(entry.Object).UnsafePointer())
		return add(key, objectOrderEntry{value: entry.Object, keys: entry.Keys})
	})
	if err != nil {
		return nil, err
	}
	for key, entry := range local {
		if err := add(key, entry); err != nil {
			return nil, err
		}
	}
	if sourceKey != 0 {
		sources[sourceKey] = captured
	}
	return result, nil
}
