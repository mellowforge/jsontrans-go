package jsonpath

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

// EvalPrepared executes singular scalar selections directly against the indexed
// document. Other queries and selected containers use the ordinary evaluator.
func (p *program) EvalPrepared(ctx context.Context, d *jsontrans.Document, o jsontrans.EvalOptions) (jsontrans.Result, error) {
	if p == nil {
		return jsontrans.Result{}, errors.New("nil prepared program")
	}
	limits, err := budget.Resolve(p.limits, o.Limits)
	if err != nil {
		return jsontrans.Result{}, err
	}
	result, err := prepared.Evaluate(ctx, d, o.Bindings, limits, o.Limits != nil || p.limits != budget.DefaultLimits(), p.evalPrepared)
	return jsontrans.Result{Value: result.Value, Defined: result.Defined}, err
}

// NativePrepared exposes an opaque capability sealed to this exact program.
func (p *program) NativePrepared() *prepared.Handler { return p.prepared }
func (p *program) evalPrepared(req prepared.Request) (prepared.Result, error) {
	if len(req.Bindings) > 0 {
		return prepared.Result{}, jsontrans.ErrUnsupported
	}
	d, b, l := req.Document, req.Budget, req.Limits
	finish := func(value any) (prepared.Result, error) {
		out, size, err := jsonvalue.CloneDetachedSize(value, b, l.MaxOutputBytes)
		if err != nil {
			return prepared.Result{}, err
		}
		return prepared.Result{Value: out, Defined: true, Bytes: size}, req.CheckExit()
	}
	fallback := func() (prepared.Result, error) {
		b.SetContext(jsonvalue.WithObjectOrder(b.Context()))
		v, err := document.MaterializeWithBudget(d, b)
		if err != nil {
			return prepared.Result{}, err
		}
		ev := evaluation{root: v, b: b, funcs: p.funcs}
		list, err := ev.evalQuery(p.query, v)
		if err != nil {
			return prepared.Result{}, err
		}
		return finish(list)
	}
	if !p.preparedSingular {
		return fallback()
	}
	var err error

	n := d.Root()
	found := true
	for _, seg := range p.query.segments {
		if err = b.Step(); err != nil {
			return prepared.Result{}, err
		}
		s := seg.sels[0]
		if s.kind == 'n' {
			n, found = n.Member(s.name)
		} else {
			i := *s.start
			if i < 0 {
				i += n.Len()
			}
			n, found = n.Index(i)
		}
		if !found {
			break
		}
	}
	values := []any{}
	if found {
		var value any
		switch n.Kind() {
		case "null":
		case "boolean":
			value, _ = n.Boolean()
		case "number":
			number, _ := n.Number()
			value = number
		case "string":
			text, _ := n.Text()
			if utf8.ValidString(text) {
				value = text
			} else {
				value = jsonvalue.UTF16String(text)
			}
		default:
			return fallback()
		}
		values = append(values, value)
	}
	return finish(values)
}

func isPreparedSingular(q *query) bool {
	for _, seg := range q.segments {
		if seg.descendant || len(seg.sels) != 1 || (seg.sels[0].kind != 'n' && seg.sels[0].kind != 'i') {
			return false
		}
	}
	return true
}
