// Package jsonpath implements JSONPath queries.
package jsonpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

type Type int

const (
	ValueType Type = iota
	LogicalType
	NodesType
)

type Function struct {
	Parameters []Type
	Result     Type
	Call       func(context.Context, []any) (any, error)
}

// Option configures New. Options act on construction state only, so they
// cannot alter an engine that is already in use.
type Option func(*options) error

type options struct {
	limits    jsontrans.Limits
	functions map[string]Function
}

type Engine struct {
	limits    jsontrans.Limits
	functions map[string]Function
}

// WithLimits sets default limits; nonzero fields refine earlier options.
func WithLimits(l jsontrans.Limits) Option {
	return func(o *options) error {
		var err error
		o.limits, err = budget.Resolve(o.limits, &l)
		return err
	}
}

// WithFunction registers a function extension. Names match [a-z][a-z0-9_]*
// and cannot replace the standard length, count, value, match or search.
func WithFunction(name string, f Function) Option {
	return func(o *options) error {
		if name == "" || f.Call == nil || f.Result < ValueType || f.Result > NodesType {
			return errors.New("invalid function")
		}
		for i, c := range name {
			if !(c >= 'a' && c <= 'z' || i > 0 && (c >= '0' && c <= '9' || c == '_')) {
				return errors.New("invalid function name")
			}
		}
		switch name {
		case "length", "count", "value", "match", "search":
			return errors.New("cannot replace standard function")
		}
		for _, t := range f.Parameters {
			if t < ValueType || t > NodesType {
				return errors.New("invalid function parameter type")
			}
		}
		f.Parameters = append([]Type(nil), f.Parameters...)
		o.functions[name] = f
		return nil
	}
}

// New returns a JSONPath (RFC 9535) engine.
func New(opts ...Option) (*Engine, error) {
	o := options{limits: budget.DefaultLimits(), functions: map[string]Function{}}
	for _, apply := range opts {
		if apply == nil {
			return nil, errors.New("nil option")
		}
		if err := apply(&o); err != nil {
			return nil, err
		}
	}
	return &Engine{limits: o.limits, functions: o.functions}, nil
}

type program struct {
	query            *query
	limits           jsontrans.Limits
	funcs            map[string]Function
	preparedSingular bool
	prepared         *prepared.Handler
}

// Compile compiles a JSONPath query. Programs return the selected node list
// and reject evaluation bindings with ErrUnsupported.
func (e *Engine) Compile(ctx context.Context, s string) (jsontrans.Program, error) {
	b, err := budget.New(ctx, e.limits)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if err = b.Expression(s); err != nil {
		return nil, err
	}
	if !utf8.ValidString(s) {
		return nil, &jsontrans.Error{Processor: "jsonpath", Phase: jsontrans.PhaseCompile, Code: "syntax", Cause: errors.New("invalid UTF-8")}
	}
	p := parser{s: s, b: b, funcs: e.functions}
	q, err := p.query(false)
	if err == nil && p.i != len(s) {
		err = p.fail("unexpected trailing input")
	}
	if err != nil {
		return nil, err
	}
	funcs := make(map[string]Function, len(e.functions))
	for name, f := range e.functions {
		f.Parameters = append([]Type(nil), f.Parameters...)
		funcs[name] = f
	}
	program := &program{query: q, limits: e.limits, funcs: funcs, preparedSingular: isPreparedSingular(q)}
	newHandler := prepared.New
	if len(funcs) == 0 {
		newHandler = prepared.NewReusable
	}
	program.prepared = newHandler(program, program.evalPrepared)
	return program, nil
}
func (p *program) Eval(ctx context.Context, input any, o jsontrans.EvalOptions) (jsontrans.Result, error) {
	if len(o.Bindings) > 0 {
		return jsontrans.Result{}, jsontrans.ErrUnsupported
	}
	l, err := budget.Resolve(p.limits, o.Limits)
	if err != nil {
		return jsontrans.Result{}, err
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return jsontrans.Result{}, err
	}
	defer b.Close()
	v, err := jsonvalue.Clone(input, b, l.MaxInputBytes)
	if err != nil {
		return jsontrans.Result{}, err
	}
	ev := evaluation{root: v, b: b, funcs: p.funcs}
	vlist, err := ev.evalQuery(p.query, v)
	if err != nil {
		return jsontrans.Result{}, err
	}
	out, err := jsonvalue.Clone(vlist, b, l.MaxOutputBytes)
	if err != nil {
		return jsontrans.Result{}, err
	}
	return jsontrans.Result{Value: out, Defined: true}, nil
}

type query struct {
	root     byte
	segments []segment
	singular bool
}
type segment struct {
	descendant bool
	sels       []selector
}
type selector struct {
	kind             byte
	name             string
	start, end, step *int
	filter           expr
}
type expr interface {
	eval(*evaluation, any) (any, error)
	typ() Type
}
type parser struct {
	s     string
	i     int
	b     *budget.Budget
	funcs map[string]Function
}

func (p *parser) fail(msg string) error {
	return &jsontrans.Error{Processor: "jsonpath", Phase: jsontrans.PhaseCompile, Code: "syntax", Position: p.i, Cause: errors.New(msg)}
}
func (p *parser) space() {
	for p.i < len(p.s) && strings.ContainsRune(" \t\r\n", rune(p.s[p.i])) {
		p.i++
	}
}
func (p *parser) take(s string) bool {
	if strings.HasPrefix(p.s[p.i:], s) {
		p.i += len(s)
		return true
	}
	return false
}
func (p *parser) query(nested bool) (*query, error) {
	if err := p.b.Enter(); err != nil {
		return nil, err
	}
	defer p.b.Leave()
	if p.i >= len(p.s) || (p.s[p.i] != '$' && (!nested || p.s[p.i] != '@')) {
		return nil, p.fail("expected query root")
	}
	q := &query{root: p.s[p.i], singular: true}
	p.i++
	for p.i < len(p.s) {
		if err := p.b.Step(); err != nil {
			return nil, err
		}
		var seg segment
		beforeSpace := p.i
		p.space()
		if p.take("..") {
			seg.descendant = true
			q.singular = false
			if p.i < len(p.s) && p.s[p.i] == '[' {
			} else if p.take("*") {
				seg.sels = []selector{{kind: '*'}}
			} else {
				name := p.name()
				if name == "" {
					return nil, p.fail("expected descendant selector")
				}
				seg.sels = []selector{{kind: 'n', name: name}}
			}
		} else if p.take(".") {
			if p.take("*") {
				seg.sels = []selector{{kind: '*'}}
				q.singular = false
			} else {
				name := p.name()
				if name == "" {
					return nil, p.fail("expected member name")
				}
				seg.sels = []selector{{kind: 'n', name: name}}
			}
		} else if p.i < len(p.s) && p.s[p.i] == '[' {
		} else {
			p.i = beforeSpace
			break
		}
		if len(seg.sels) == 0 {
			p.i++
			p.space()
			for {
				if err := p.b.Step(); err != nil {
					return nil, err
				}
				s, err := p.selector()
				if err != nil {
					return nil, err
				}
				seg.sels = append(seg.sels, s)
				if s.kind != 'n' && s.kind != 'i' {
					q.singular = false
				}
				p.space()
				if p.take("]") {
					break
				}
				if !p.take(",") {
					return nil, p.fail("expected comma or closing bracket")
				}
				p.space()
			}
			if len(seg.sels) > 1 {
				q.singular = false
			}
		}
		q.segments = append(q.segments, seg)
	}
	return q, nil
}
func (p *parser) name() string {
	start := p.i
	for p.i < len(p.s) {
		r, n := utf8.DecodeRuneInString(p.s[p.i:])
		if r == utf8.RuneError && n == 1 {
			break
		}
		if !(r == '_' || r > 127 || unicode.IsLetter(r) || (p.i > start && unicode.IsDigit(r))) {
			break
		}
		p.i += n
	}
	return p.s[start:p.i]
}
func (p *parser) str() (string, error) {
	if p.i >= len(p.s) || (p.s[p.i] != '\'' && p.s[p.i] != '"') {
		return "", p.fail("expected string")
	}
	quote := p.s[p.i]
	p.i++
	var out strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		if c == quote {
			return out.String(), nil
		}
		if c < 0x20 {
			return "", p.fail("control in string")
		}
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		if p.i >= len(p.s) {
			break
		}
		c = p.s[p.i]
		p.i++
		switch c {
		case '\'', '"':
			if c != quote {
				return "", p.fail("invalid escape")
			}
			out.WriteByte(c)
		case '\\', '/':
			out.WriteByte(c)
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'u':
			if p.i+4 > len(p.s) {
				return "", p.fail("bad unicode escape")
			}
			h := p.s[p.i : p.i+4]
			p.i += 4
			v, err := strconv.ParseUint(h, 16, 16)
			if err != nil {
				return "", p.fail("bad unicode escape")
			}
			r := rune(v)
			if r >= 0xd800 && r <= 0xdbff {
				if !p.take(`\u`) || p.i+4 > len(p.s) {
					return "", p.fail("bad surrogate")
				}
				v2, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 16)
				p.i += 4
				if err != nil || v2 < 0xdc00 || v2 > 0xdfff {
					return "", p.fail("bad surrogate")
				}
				r = 0x10000 + (r-0xd800)*0x400 + rune(v2) - 0xdc00
			} else if r >= 0xdc00 && r <= 0xdfff {
				return "", p.fail("bad surrogate")
			}
			out.WriteRune(r)
		default:
			return "", p.fail("invalid escape")
		}
	}
	return "", p.fail("unterminated string")
}
func (p *parser) integer() (*int, error) {
	start := p.i
	p.take("-")
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if p.i == start {
		return nil, nil
	}
	s := p.s[start:p.i]
	if s == "-" || s == "-0" || len(s) > 1 && s[0] == '0' || strings.HasPrefix(s, "-0") {
		return nil, p.fail("invalid integer")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n > 9007199254740991 || n < -9007199254740991 {
		return nil, p.fail("integer out of range")
	}
	v := int(n)
	return &v, nil
}
func (p *parser) selector() (selector, error) {
	if p.i >= len(p.s) {
		return selector{}, p.fail("missing selector")
	}
	switch p.s[p.i] {
	case '\'', '"':
		s, e := p.str()
		return selector{kind: 'n', name: s}, e
	case '*':
		p.i++
		return selector{kind: '*'}, nil
	case '?':
		p.i++
		p.space()
		x, e := p.expression()
		if e != nil {
			return selector{}, e
		}
		if x.typ() != LogicalType && x.typ() != NodesType {
			return selector{}, p.fail("filter must be logical")
		}
		return selector{kind: '?', filter: x}, nil
	}
	a, e := p.integer()
	if e != nil {
		return selector{}, e
	}
	p.space()
	if p.take(":") {
		p.space()
		b, e := p.integer()
		if e != nil {
			return selector{}, e
		}
		var c *int
		p.space()
		if p.take(":") {
			p.space()
			c, e = p.integer()
			if e != nil {
				return selector{}, e
			}
		}
		return selector{kind: 's', start: a, end: b, step: c}, nil
	}
	if a == nil {
		return selector{}, p.fail("invalid selector")
	}
	return selector{kind: 'i', start: a}, nil
}

type evaluation struct {
	root  any
	b     *budget.Budget
	funcs map[string]Function
}

func (e *evaluation) evalQuery(q *query, current any) ([]any, error) {
	if err := e.b.Enter(); err != nil {
		return nil, err
	}
	defer e.b.Leave()
	root := current
	if q.root == '$' {
		root = e.root
	}
	nodes := []any{root}
	for _, seg := range q.segments {
		out := []any{}
		var visit func(any) error
		visit = func(v any) error {
			if err := e.b.Enter(); err != nil {
				return err
			}
			defer e.b.Leave()
			for _, s := range seg.sels {
				v2, err := e.selectNode(s, v)
				if err != nil {
					return err
				}
				out = append(out, v2...)
				if err = e.b.Collection(len(out)); err != nil {
					return err
				}
			}
			if seg.descendant {
				children := children(v)
				for _, c := range children {
					if err := visit(c); err != nil {
						return err
					}
				}
			}
			return nil
		}
		for _, v := range nodes {
			if err := visit(v); err != nil {
				return nil, err
			}
		}
		nodes = out
	}
	return nodes, nil
}

// Strings and keys containing isolated surrogates arrive as lossless
// UTF16String/UTF16Object wrappers; to JSONPath they are ordinary JSON values.
func asString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case jsonvalue.UTF16String:
		return string(x), true
	}
	return "", false
}
func asObject(v any) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		return x, true
	case jsonvalue.UTF16Object:
		return map[string]any(x), true
	}
	return nil, false
}

// characterCount counts code points, treating each isolated surrogate as one.
func characterCount(s string) int {
	n := 0
	for len(s) > 0 {
		_, size := jsonvalue.DecodeRune(s)
		s = s[size:]
		n++
	}
	return n
}

// matchSubject lets RE2 consume each isolated surrogate as one character.
func matchSubject(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	for len(s) > 0 {
		r, size := jsonvalue.DecodeRune(s)
		if r >= 0xd800 && r <= 0xdfff {
			r = utf8.RuneError
		}
		out = utf8.AppendRune(out, r)
		s = s[size:]
	}
	return string(out)
}
func children(v any) []any {
	if x, ok := asObject(v); ok {
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(x))
		for _, k := range keys {
			out = append(out, x[k])
		}
		return out
	}
	if x, ok := v.([]any); ok {
		return x
	}
	return nil
}
func (e *evaluation) selectNode(s selector, v any) ([]any, error) {
	if err := e.b.Step(); err != nil {
		return nil, err
	}
	out := []any{}
	switch s.kind {
	case 'n':
		if m, ok := asObject(v); ok {
			if x, ok := m[s.name]; ok {
				out = append(out, x)
			}
		}
	case '*':
		out = append(out, children(v)...)
	case 'i':
		if a, ok := v.([]any); ok {
			i := *s.start
			if i < 0 {
				i += len(a)
			}
			if i >= 0 && i < len(a) {
				out = append(out, a[i])
			}
		}
	case 's':
		if a, ok := v.([]any); ok {
			n := len(a)
			step := 1
			if s.step != nil {
				step = *s.step
			}
			if step == 0 {
				break
			}
			start, end := 0, n
			if step < 0 {
				start = n - 1
				end = -1
			}
			if s.start != nil {
				start = *s.start
				if start < 0 {
					start += n
				}
			}
			if s.end != nil {
				end = *s.end
				if end < 0 {
					end += n
				}
			}
			if step > 0 {
				if start < 0 {
					start = 0
				}
				if start > n {
					start = n
				}
				if end < 0 {
					end = 0
				}
				if end > n {
					end = n
				}
				for i := start; i < end; i += step {
					if err := e.b.Step(); err != nil {
						return nil, err
					}
					out = append(out, a[i])
				}
			} else {
				if start < -1 {
					start = -1
				}
				if start >= n {
					start = n - 1
				}
				if end < -1 {
					end = -1
				}
				if end >= n {
					end = n - 1
				}
				for i := start; i > end; i += step {
					if err := e.b.Step(); err != nil {
						return nil, err
					}
					out = append(out, a[i])
				}
			}
		}
	case '?':
		for _, c := range children(v) {
			x, err := s.filter.eval(e, c)
			if err != nil {
				return nil, err
			}
			if truth(x) {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

type literal struct{ v any }

func (x literal) eval(_ *evaluation, _ any) (any, error) { return x.v, nil }
func (literal) typ() Type                                { return ValueType }

type queryExpr struct{ q *query }

func (x queryExpr) eval(e *evaluation, c any) (any, error) {
	v, err := e.evalQuery(x.q, c)
	return Nodes(v), err
}
func (queryExpr) typ() Type { return NodesType }

// Nodes is the node-list value passed to and returned by extension functions.
type Nodes []any

// Nothing represents the RFC Nothing value in extension functions.
type Nothing struct{}

func value(v any) any {
	if n, ok := v.(Nodes); ok {
		if len(n) == 1 {
			return n[0]
		}
		return Nothing{}
	}
	return v
}
func truth(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case Nodes:
		return len(x) > 0
	}
	return false
}

type unary struct{ x expr }

func (x unary) typ() Type { return LogicalType }
func (x unary) eval(e *evaluation, c any) (any, error) {
	v, err := x.x.eval(e, c)
	return !truth(v), err
}

type binary struct {
	op   string
	a, b expr
}

func (binary) typ() Type { return LogicalType }
func (x binary) eval(e *evaluation, c any) (any, error) {
	if err := e.b.Enter(); err != nil {
		return nil, err
	}
	defer e.b.Leave()
	a, err := x.a.eval(e, c)
	if err != nil {
		return nil, err
	}
	if x.op == "&&" && !truth(a) {
		return false, nil
	}
	if x.op == "||" && truth(a) {
		return true, nil
	}
	b, err := x.b.eval(e, c)
	if err != nil {
		return nil, err
	}
	switch x.op {
	case "&&":
		return truth(b), nil
	case "||":
		return truth(b), nil
	}
	a = value(a)
	b = value(b)
	if err := chargeComparison(e.b, a); err != nil {
		return nil, err
	}
	if err := chargeComparison(e.b, b); err != nil {
		return nil, err
	}
	cmp, ordered := compare(a, b)
	eq := equal(a, b)
	switch x.op {
	case "==":
		return eq, nil
	case "!=":
		return !eq, nil
	case "<":
		return ordered && cmp < 0, nil
	case "<=":
		return eq || ordered && cmp < 0, nil
	case ">":
		return ordered && cmp > 0, nil
	case ">=":
		return eq || ordered && cmp > 0, nil
	}
	return false, nil
}

type decimal struct {
	sign     int
	digits   string
	exponent string
}

func magnitudeCmp(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

// addDecimal adds two signed decimal integers in linear time.
func addDecimal(a, b string) string {
	negA, negB := strings.HasPrefix(a, "-"), strings.HasPrefix(b, "-")
	a = strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "+"), "0")
	b = strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(b, "-"), "+"), "0")
	if a == "" {
		a = "0"
		negA = false
	}
	if b == "" {
		b = "0"
		negB = false
	}
	negative := negA
	if negA != negB && magnitudeCmp(a, b) < 0 {
		a, b = b, a
		negative = negB
	}
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]byte, n+1)
	carry := 0
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(a) {
			x = int(a[len(a)-1-i] - '0')
		}
		if i < len(b) {
			y = int(b[len(b)-1-i] - '0')
		}
		if negA == negB {
			v := x + y + carry
			out[n-i] = byte(v%10) + '0'
			carry = v / 10
		} else {
			v := x - y - carry
			if v < 0 {
				v += 10
				carry = 1
			} else {
				carry = 0
			}
			out[n-i] = byte(v) + '0'
		}
	}
	out[0] = byte(carry) + '0'
	z := strings.TrimLeft(string(out), "0")
	if z == "" {
		return "0"
	}
	if negative {
		return "-" + z
	}
	return z
}
func exponentCmp(a, b string) int {
	na, nb := strings.HasPrefix(a, "-"), strings.HasPrefix(b, "-")
	if na != nb {
		if na {
			return -1
		}
		return 1
	}
	c := magnitudeCmp(strings.TrimPrefix(a, "-"), strings.TrimPrefix(b, "-"))
	if na {
		return -c
	}
	return c
}
func number(v any) *decimal {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = string(x)
	case float64:
		s = strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(x), 'g', -1, 32)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		s = fmt.Sprint(x)
	default:
		return nil
	}
	d := &decimal{sign: 1, exponent: "0"}
	if strings.HasPrefix(s, "-") {
		d.sign = -1
		s = s[1:]
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		d.exponent = s[i+1:]
		s = s[:i]
	}
	shift := 0
	point := strings.IndexByte(s, '.')
	if point >= 0 {
		shift -= len(s) - point - 1
		s = s[:point] + s[point+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		d.sign = 0
		d.digits = "0"
		return d
	}
	d.digits = s
	d.exponent = addDecimal(d.exponent, strconv.Itoa(shift+len(s)))
	return d
}
func compare(a, b any) (int, bool) {
	if x := number(a); x != nil {
		if y := number(b); y != nil {
			if x.sign != y.sign {
				if x.sign < y.sign {
					return -1, true
				}
				return 1, true
			}
			if x.sign == 0 {
				return 0, true
			}
			if c := exponentCmp(x.exponent, y.exponent); c != 0 {
				return c * x.sign, true
			}
			n := len(x.digits)
			if len(y.digits) > n {
				n = len(y.digits)
			}
			for i := 0; i < n; i++ {
				a, b := byte('0'), byte('0')
				if i < len(x.digits) {
					a = x.digits[i]
				}
				if i < len(y.digits) {
					b = y.digits[i]
				}
				if a < b {
					return -x.sign, true
				}
				if a > b {
					return x.sign, true
				}
			}
			return 0, true
		}
	}
	if x, ok := asString(a); ok {
		if y, ok := asString(b); ok {
			return strings.Compare(x, y), true
		}
	}
	return 0, false
}
func equal(a, b any) bool {
	if c, ok := compare(a, b); ok {
		return c == 0
	}
	if x, ok := asObject(a); ok {
		y, ok := asObject(b)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equal(v, w) {
				return false
			}
		}
		return true
	}
	switch x := a.(type) {
	case Nothing:
		_, ok := b.(Nothing)
		return ok
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	return false
}

type call struct {
	name string
	args []expr
	t    Type
}

func (x call) typ() Type { return x.t }
func (x call) eval(e *evaluation, c any) (any, error) {
	if err := e.b.Step(); err != nil {
		return nil, err
	}
	a := make([]any, len(x.args))
	for i, arg := range x.args {
		v, err := arg.eval(e, c)
		if err != nil {
			return nil, err
		}
		a[i] = v
	}
	switch x.name {
	case "count":
		return json.Number(strconv.Itoa(len(a[0].(Nodes)))), nil
	case "value":
		return value(a[0]), nil
	case "length":
		v := value(a[0])
		if s, ok := asString(v); ok {
			return json.Number(strconv.Itoa(characterCount(s))), nil
		}
		if m, ok := asObject(v); ok {
			return json.Number(strconv.Itoa(len(m))), nil
		}
		if l, ok := v.([]any); ok {
			return json.Number(strconv.Itoa(len(l))), nil
		}
		return Nothing{}, nil
	case "match", "search":
		started := time.Now()
		s, ok := asString(value(a[0]))
		if !ok {
			return false, nil
		}
		s = matchSubject(s)
		pat, ok := asString(value(a[1]))
		if !ok {
			return false, nil
		}
		valid, err := validIRegexp(pat, e.b)
		if err != nil {
			return nil, err
		}
		if !valid {
			return false, nil
		}
		pat, err = expandRepeats(pat, e.b)
		if err != nil {
			return nil, err
		}
		pat = translateDots(pat)
		if x.name == "match" {
			pat = "^(?:" + pat + ")$"
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			var se *syntax.Error
			if errors.As(err, &se) && (se.Code == syntax.ErrLarge || se.Code == syntax.ErrNestingDepth) {
				return nil, fmt.Errorf("%w: regular expression complexity", budget.ErrLimit)
			}
			return false, nil
		}
		if err = e.b.Step(); err != nil {
			return nil, err
		}
		matched := re.MatchString(s)
		if err := e.b.Check(); err != nil {
			return nil, err
		}
		if time.Since(started) > e.b.Limits.RegexTimeout {
			return nil, context.DeadlineExceeded
		}
		return matched, nil
	}
	f := e.funcs[x.name]
	for i, t := range f.Parameters {
		if t == ValueType {
			a[i] = value(a[i])
		} else if t == LogicalType {
			a[i] = truth(a[i])
		}
	}
	result, err := f.Call(e.b.Context(), a)
	if err != nil {
		return nil, err
	}
	if err = e.b.Check(); err != nil {
		return nil, err
	}
	switch f.Result {
	case NodesType:
		nodes, ok := result.(Nodes)
		if !ok {
			return nil, errors.New("extension returned invalid node list")
		}
		if err := e.b.Collection(len(nodes)); err != nil {
			return nil, err
		}
		detached, err := jsonvalue.Clone(append([]any{}, nodes...), e.b, e.b.Limits.MaxInputBytes)
		if err != nil {
			return nil, err
		}
		result = Nodes(detached.([]any))
	case LogicalType:
		if _, ok := result.(bool); !ok {
			return nil, errors.New("extension returned invalid logical value")
		}
	case ValueType:
		if _, ok := result.(Nothing); !ok {
			result, err = jsonvalue.Clone(result, e.b, e.b.Limits.MaxInputBytes)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, errors.New("extension returned unknown type")
	}
	return result, nil
}
func (p *parser) expression() (expr, error) { return p.or() }
func (p *parser) or() (expr, error) {
	a, err := p.and()
	if err != nil {
		return nil, err
	}
	for {
		p.space()
		if !p.take("||") {
			return a, nil
		}
		if a.typ() == ValueType {
			return nil, p.fail("invalid logical operand")
		}
		p.space()
		b, err := p.and()
		if err != nil {
			return nil, err
		}
		if b.typ() == ValueType {
			return nil, p.fail("invalid logical operand")
		}
		a = binary{"||", a, b}
	}
}
func (p *parser) and() (expr, error) {
	a, err := p.comparison()
	if err != nil {
		return nil, err
	}
	for {
		p.space()
		if !p.take("&&") {
			return a, nil
		}
		if a.typ() == ValueType {
			return nil, p.fail("invalid logical operand")
		}
		p.space()
		b, err := p.comparison()
		if err != nil {
			return nil, err
		}
		if b.typ() == ValueType {
			return nil, p.fail("invalid logical operand")
		}
		a = binary{"&&", a, b}
	}
}
func singular(x expr) bool {
	if q, ok := x.(queryExpr); ok {
		return q.q.singular
	}
	return x.typ() == ValueType
}
func (p *parser) comparison() (expr, error) {
	a, err := p.primary()
	if err != nil {
		return nil, err
	}
	p.space()
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if p.take(op) {
			if !singular(a) {
				return nil, p.fail("comparison requires value")
			}
			p.space()
			b, err := p.primary()
			if err != nil {
				return nil, err
			}
			if !singular(b) {
				return nil, p.fail("comparison requires value")
			}
			return binary{op, a, b}, nil
		}
	}
	return a, nil
}
func (p *parser) primary() (expr, error) {
	if err := p.b.Enter(); err != nil {
		return nil, err
	}
	defer p.b.Leave()
	p.space()
	if p.take("!") {
		p.space()
		if p.i < len(p.s) && p.s[p.i] == '!' {
			return nil, p.fail("repeated negation")
		}
		x, err := p.primary()
		if err != nil {
			return nil, err
		}
		if x.typ() == ValueType {
			return nil, p.fail("invalid negation")
		}
		return unary{x}, nil
	}
	if p.take("(") {
		x, err := p.expression()
		if err != nil {
			return nil, err
		}
		p.space()
		if !p.take(")") {
			return nil, p.fail("missing closing parenthesis")
		}
		if x.typ() == ValueType {
			return nil, p.fail("parentheses require logical expression")
		}
		return grouped{x}, nil
	}
	if p.i >= len(p.s) {
		return nil, p.fail("missing expression")
	}
	switch p.s[p.i] {
	case '$', '@':
		q, err := p.query(true)
		if err != nil {
			return nil, err
		}
		return queryExpr{q}, nil
	case '\'', '"':
		s, err := p.str()
		return literal{s}, err
	}
	for _, s := range []string{"true", "false", "null"} {
		if p.take(s) {
			switch s {
			case "true":
				return literal{true}, nil
			case "false":
				return literal{false}, nil
			default:
				return literal{nil}, nil
			}
		}
	}
	if p.s[p.i] == '-' || p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		start := p.i
		p.take("-")
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
		if p.take(".") {
			for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
				p.i++
			}
		}
		if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
			p.i++
			if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
				p.i++
			}
			for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
				p.i++
			}
		}
		s := p.s[start:p.i]
		if !json.Valid([]byte(s)) {
			return nil, p.fail("invalid number")
		}
		return literal{json.Number(s)}, nil
	}
	name := p.name()
	if name == "" || !p.take("(") {
		return nil, p.fail("expected function")
	}
	params := []Type{}
	result := ValueType
	switch name {
	case "length":
		params = []Type{ValueType}
	case "count":
		params = []Type{NodesType}
	case "value":
		params = []Type{NodesType}
	case "match", "search":
		params = []Type{ValueType, ValueType}
		result = LogicalType
	default:
		f, ok := p.funcs[name]
		if !ok {
			return nil, p.fail("unknown function")
		}
		params = f.Parameters
		result = f.Result
	}
	args := []expr{}
	p.space()
	if !p.take(")") {
		for {
			a, err := p.expression()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			p.space()
			if p.take(")") {
				break
			}
			if !p.take(",") {
				return nil, p.fail("expected argument separator")
			}
			p.space()
		}
	}
	if len(args) != len(params) {
		return nil, p.fail("incorrect argument count")
	}
	for i, a := range args {
		if params[i] == ValueType && !singular(a) || params[i] == NodesType && a.typ() != NodesType || params[i] == LogicalType && a.typ() == ValueType {
			return nil, p.fail("incorrect argument type")
		}
	}
	return call{name, args, result}, nil
}

func translateDots(s string) string {
	var b strings.Builder
	escaped, class := false, false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			b.WriteRune(r)
			escaped = true
			continue
		}
		if r == '[' {
			class = true
		}
		if r == ']' {
			class = false
		}
		if r == '.' && !class {
			b.WriteString(`[^\r\n]`)
		} else if !class && (r == '^' || r == '$') {
			b.WriteByte('\\')
			b.WriteRune(r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// validIRegexp checks the RFC 9485 grammar before handing the translated
// pattern to Go's regexp compiler, which otherwise accepts RE2 extensions.
func validIRegexp(s string, b *budget.Budget) (bool, error) {
	depth := 0
	atom := false
	quantified := false
	categories := map[string]bool{}
	for _, x := range strings.Fields("L Ll Lm Lo Lt Lu M Mc Me Mn N Nd Nl No P Pc Pd Pe Pf Pi Po Ps Z Zl Zp Zs S Sc Sk Sm So C Cc Cf Cn Co") {
		categories[x] = true
	}
	for i := 0; i < len(s); {
		if err := b.Step(); err != nil {
			return false, err
		}
		c := s[i]
		i++
		switch c {
		case '\\':
			if i >= len(s) {
				return false, nil
			}
			x := s[i]
			i++
			if x == 'p' || x == 'P' {
				if i >= len(s) || s[i] != '{' {
					return false, nil
				}
				i++
				start := i
				for i < len(s) && s[i] != '}' {
					i++
				}
				if i >= len(s) || !categories[s[start:i]] {
					return false, nil
				}
				i++
			} else if !strings.ContainsRune("()*+-.?[]^nrt{}|\\", rune(x)) {
				return false, nil
			}
			atom = true
			quantified = false
		case '[':
			end, ok, err := parseIRegexpClass(s, i, categories, b)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
			i = end
			atom = true
			quantified = false
		case '(':
			depth++
			if depth > b.Limits.MaxDepth {
				return false, budget.ErrLimit
			}
			atom = false
			quantified = false
		case ')':
			if depth == 0 {
				return false, nil
			}
			depth--
			atom = true
			quantified = false
		case '|':
			atom = false
			quantified = false
		case '*', '+', '?':
			if !atom || quantified {
				return false, nil
			}
			quantified = true
		case '{':
			if !atom || quantified {
				return false, nil
			}
			start := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i == start {
				return false, nil
			}
			if i < len(s) && s[i] == ',' {
				i++
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
			}
			if i >= len(s) || s[i] != '}' {
				return false, nil
			}
			i++
			quantified = true
		case '}', ']':
			return false, nil
		default:
			atom = true
			quantified = false
		}
	}
	return depth == 0, nil
}

func chargeComparison(b *budget.Budget, v any) error {
	if err := b.Enter(); err != nil {
		return err
	}
	defer b.Leave()
	n := 0
	if s, ok := asString(v); ok {
		n = len(s)
	} else if m, ok := asObject(v); ok {
		for k, v := range m {
			n += len(k)
			if err := chargeComparison(b, v); err != nil {
				return err
			}
		}
	}
	switch x := v.(type) {
	case json.Number:
		n = len(x)
	case []any:
		for _, v := range x {
			if err := chargeComparison(b, v); err != nil {
				return err
			}
		}
	}
	for n > 0 {
		if err := b.Step(); err != nil {
			return err
		}
		n -= 256
	}
	return nil
}

// Limits returns the configured default limits for this engine.
func (e *Engine) Limits() jsontrans.Limits { return e.limits }

// Limits returns the configured default limits for this program.
func (p *program) Limits() jsontrans.Limits { return p.limits }

// expandRepeats lowers large I-Regexp counters to RE2's bounded counters.
func expandRepeats(s string, b *budget.Budget) (string, error) {
	var out strings.Builder
	stack := []int{}
	atom := 0
	for i := 0; i < len(s); {
		if err := b.Step(); err != nil {
			return "", err
		}
		c := s[i]
		start := i
		i++
		switch c {
		case '\\':
			atom = out.Len()
			if i < len(s) {
				x := s[i]
				i++
				if (x == 'p' || x == 'P') && i < len(s) && s[i] == '{' {
					for i < len(s) && s[i] != '}' {
						i++
					}
					if i < len(s) {
						i++
					}
				}
			}
			out.WriteString(s[start:i])
		case '[':
			atom = out.Len()
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == ']' {
					i++
					break
				}
				i++
			}
			if i > len(s) {
				i = len(s)
			}
			out.WriteString(s[start:i])
		case '(':
			stack = append(stack, out.Len())
			out.WriteByte(c)
		case ')':
			out.WriteByte(c)
			atom = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case '{':
			a := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			lo, err := strconv.ParseInt(s[a:i], 10, 64)
			if err != nil {
				return "", budget.ErrLimit
			}
			hi := lo
			unbounded := false
			if i < len(s) && s[i] == ',' {
				i++
				a = i
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				if i == a {
					unbounded = true
				} else {
					hi, err = strconv.ParseInt(s[a:i], 10, 64)
					if err != nil {
						return "", budget.ErrLimit
					}
				}
			}
			i++
			if !unbounded && hi < lo {
				out.WriteString(s[start:i])
				continue
			}
			if lo > int64(b.Limits.MaxSteps) || (!unbounded && hi > int64(b.Limits.MaxSteps)) {
				return "", budget.ErrLimit
			}
			prev := out.String()
			piece := prev[atom:]
			out.Reset()
			out.WriteString(prev[:atom])
			emit := func(count int64, optional bool) error {
				for j := int64(0); j < count; j++ {
					if err := b.Step(); err != nil {
						return err
					}
					if out.Len()+len(piece)+4 > b.Limits.MaxExpressionBytes {
						return budget.ErrLimit
					}
					if optional {
						out.WriteString("(?:")
						out.WriteString(piece)
						out.WriteString(")?")
					} else {
						out.WriteString(piece)
					}
				}
				return nil
			}
			if err := emit(lo, false); err != nil {
				return "", err
			}
			if unbounded {
				out.WriteString("(?:")
				out.WriteString(piece)
				out.WriteString(")*")
			} else if err := emit(hi-lo, true); err != nil {
				return "", err
			}

		case '*', '+', '?', '|':
			out.WriteByte(c)
		default:
			atom = out.Len()
			if c >= utf8.RuneSelf {
				_, n := utf8.DecodeRuneInString(s[start:])
				if n > 1 {
					i = start + n
				}
				out.WriteString(s[start:i])
			} else {
				out.WriteByte(c)
			}
		}
	}
	return out.String(), nil
}

// grouped keeps parenthesized expressions logical even when their operand is a query.
type grouped struct{ x expr }

func (grouped) typ() Type { return LogicalType }
func (x grouped) eval(e *evaluation, c any) (any, error) {
	v, err := x.x.eval(e, c)
	return truth(v), err
}

// parseIRegexpClass parses RFC 9485 charClassExpr and its CCE1 elements.
// i points after the opening bracket; the returned offset follows its closing bracket.
func parseIRegexpClass(s string, i int, categories map[string]bool, b *budget.Budget) (int, bool, error) {
	if i < len(s) && s[i] == '^' {
		i++
	}
	count := 0
	if i < len(s) && s[i] == '-' {
		i++
		count++
	}
	// A token is either a single character usable as a range endpoint or a category.
	token := func() (bool, bool, error) {
		if err := b.Step(); err != nil {
			return false, false, err
		}
		if i >= len(s) {
			return false, false, nil
		}
		switch s[i] {
		case '\\':
			i++
			if i >= len(s) {
				return false, false, nil
			}
			x := s[i]
			i++
			if x == 'p' || x == 'P' {
				if i >= len(s) || s[i] != '{' {
					return false, false, nil
				}
				i++
				start := i
				for i < len(s) && s[i] != '}' {
					if err := b.Step(); err != nil {
						return false, false, err
					}
					i++
				}
				if i >= len(s) || !categories[s[start:i]] {
					return false, false, nil
				}
				i++
				return true, true, nil
			}
			if !strings.ContainsRune("()*+-.?[]^nrt{}|\\", rune(x)) {
				return false, false, nil
			}
			return false, true, nil
		case '[', ']', '-':
			return false, false, nil
		default:
			_, n := utf8.DecodeRuneInString(s[i:])
			if n == 1 && s[i] >= utf8.RuneSelf {
				return false, false, nil
			}
			i += n
			return false, true, nil
		}
	}
	for i < len(s) && s[i] != ']' {
		if s[i] == '-' {
			if i+1 < len(s) && s[i+1] == ']' && count > 0 {
				i++
				break
			}
			return 0, false, nil
		}
		category, ok, err := token()
		if err != nil || !ok {
			return 0, false, err
		}
		count++
		if i < len(s) && s[i] == '-' && i+1 < len(s) && s[i+1] != ']' {
			if category {
				return 0, false, nil
			}
			i++
			category, ok, err = token()
			if err != nil || !ok || category {
				return 0, false, err
			}
		}
	}
	if count == 0 || i >= len(s) || s[i] != ']' {
		return 0, false, nil
	}
	return i + 1, true, nil
}
