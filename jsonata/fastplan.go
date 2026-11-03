package jsonata

import (
	"math"
	"reflect"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

// fastPlan retains only immutable syntax. Unsupported runtime shapes fall back.
type fastPlan struct {
	root            *node
	projection      []*node
	sumProjection   bool
	memoIDs         map[*node]int
	memoPrimary     *node
	memoPrimaryID   int
	numericPath     *node
	numericLiteral  float64
	numericReversed bool
}

func compileFastPlan(n *node) *fastPlan {
	var pathSupported func(*node) bool
	pathSupported = func(n *node) bool {
		switch n.op {
		case "name":
			return true
		case "variable":
			return n.val == "$" || n.val == "$$"
		case ".":
			return (n.children[1].op == "name" || n.children[1].op == "filter") && pathSupported(n.children[0]) && pathSupported(n.children[1])
		case "filter":
			return pathSupported(n.children[0]) && n.children[1].op == "number" && n.children[1].val.(float64) >= 0 && n.children[1].val.(float64) < 1e9
		}
		return false
	}
	rootNode := n
	var supported func(*node) bool
	supported = func(n *node) bool {
		switch n.op {
		case "name", "number", "string", "literal":
			return true
		case "variable":
			return n.val == "$" || n.val == "$$"
		case ".":
			if n == rootNode && n.children[0].op == "filter" && n.children[0].children[0].op == "name" && n.children[1].op == "name" {
				predicate := n.children[0].children[1]
				switch predicate.op {
				case "=", "!=", "<", ">", "<=", ">=", "and", "or":
					return fastProjectionPredicate(predicate)
				}
			}
			return pathSupported(n)
		case "filter":
			return pathSupported(n)
		case "=", "!=", "<", ">", "<=", ">=", "and", "or":
			return supported(n.children[0]) && supported(n.children[1])
		case "call":
			if len(n.children) != 2 || n.children[0].op != "variable" {
				return false
			}
			switch n.children[0].val.(string) {
			case "$exists", "$type", "$count", "$length", "$sum":
				return supported(n.children[1])
			}
		}
		return false
	}
	projectionRoot := n
	sumProjection := n.op == "call" && len(n.children) == 2 && n.children[0].op == "variable" && n.children[0].val == "$sum"
	if sumProjection {
		projectionRoot = n.children[1]
	}
	projection := compileProjection(projectionRoot, sumProjection)
	if !supported(n) && projection == nil {
		return nil
	}
	// The fast walk adds accounting frames for path operations. Leave ample
	// headroom under the default evaluator depth; deeper syntax falls back.
	var depth func(*node) int
	depth = func(n *node) int {
		d := 0
		for _, child := range n.children {
			if c := depth(child); c > d {
				d = c
			}
		}
		return d + 1
	}
	if depth(n) > 120 {
		return nil
	}
	plan := &fastPlan{root: n, projection: projection, sumProjection: sumProjection && projection != nil}
	switch n.op {
	case "=", "!=", "<", ">", "<=", ">=":
		if n.children[1].op == "number" && pathSupported(n.children[0]) {
			plan.numericPath = n.children[0]
			plan.numericLiteral = n.children[1].val.(float64)
		} else if n.children[0].op == "number" && pathSupported(n.children[1]) {
			plan.numericPath = n.children[1]
			plan.numericLiteral = n.children[0].val.(float64)
			plan.numericReversed = true
		}
	}
	return plan
}

func (p *fastPlan) eval(root jsontrans.Node, bd *budget.Budget) (jsontrans.Result, bool, error) {
	return p.evalWithMemo(root, bd, nil)
}

func (p *fastPlan) evalWithMemo(root jsontrans.Node, bd *budget.Budget, memo *batchMemo) (jsontrans.Result, bool, error) {
	if p.sumProjection {
		return p.evalProjection(root, bd)
	}
	var evalErr error
	var path func(*node, jsontrans.Node) (jsontrans.Node, bool, bool)
	path = func(n *node, focus jsontrans.Node) (jsontrans.Node, bool, bool) {
		if memo != nil && focus == root {
			id := p.memoPrimaryID
			if n != p.memoPrimary {
				id = p.memoIDs[n]
			}
			if id != 0 {
				var v jsontrans.Node
				var found, ok bool
				v, found, ok, evalErr = memo.resolve(id, bd)
				return v, found, ok
			}
		}
		if evalErr = bd.Enter(); evalErr != nil {
			return jsontrans.Node{}, false, false
		}
		defer bd.Leave()
		if n.op == "variable" {
			return focus, true, true
		}
		if n.op == "filter" {
			if n.children[1].op != "number" {
				return jsontrans.Node{}, false, false
			}
			v, defined, ok := path(n.children[0], focus)
			if !ok || !defined {
				return v, defined, ok
			}
			idx := int(n.children[1].val.(float64))
			if v.Kind() == "array" {
				v, defined = v.Index(idx)
				return v, defined, true
			}
			if idx == 0 {
				return v, true, true
			}
			return jsontrans.Node{}, false, true
		}
		if n.op == "." {
			v, defined, ok := path(n.children[0], focus)
			if !ok || !defined {
				return v, defined, ok
			}
			return path(n.children[1], v)
		}
		if focus.Kind() == "array" {
			return jsontrans.Node{}, false, false
		}
		v, found := focus.Member(n.val.(string))
		return v, found, true
	}
	// Keep numeric field comparisons in binary64 registers until their boolean
	// result. Boxing a document number into Value costs one allocation per rule.
	if p.numericPath != nil {
		if err := bd.Enter(); err != nil {
			return jsontrans.Result{}, false, err
		}
		value, found, handled := path(p.numericPath, root)
		bd.Leave()
		if evalErr != nil {
			return jsontrans.Result{}, false, evalErr
		}

		if handled && found {
			if a, ok := value.Binary64(); ok {
				b := p.numericLiteral
				if p.numericReversed {
					a, b = b, a
				}
				var result bool
				switch p.root.op {
				case "=":
					result = a == b
				case "!=":
					result = a != b
				case "<":
					result = a < b
				case ">":
					result = a > b
				case "<=":
					result = a <= b
				case ">=":
					result = a >= b
				}
				return jsontrans.Result{Value: result, Defined: true}, true, nil
			}
		}
	}
	var eval func(*node) (Value, bool)
	eval = func(n *node) (Value, bool) {
		if evalErr = bd.Enter(); evalErr != nil {
			return nil, false
		}
		defer bd.Leave()
		switch n.op {
		case "name", ".", "filter", "variable":
			if n == p.root && n.op == "." && n.children[1].op == "name" && (n.children[0].op == "name" || (n.children[0].op == "filter" && n.children[0].children[0].op == "name" && n.children[0].children[1].op != "number")) {
				left := n.children[0]
				var predicate *node
				if left.op == "filter" {
					predicate = left.children[1]
					left = left.children[0]
				}
				source, found := root.Member(left.val.(string))
				if found && source.Kind() == "array" {
					var out []any
					for i := 0; i < source.Len(); i++ {
						if evalErr = bd.Step(); evalErr != nil {
							return nil, false
						}
						item, _ := source.Index(i)
						if item.Kind() == "array" {
							return nil, false
						}
						if predicate != nil {
							result, handled, err := (&fastPlan{root: predicate}).eval(item, bd)
							if err != nil {
								evalErr = err
								return nil, false
							}
							if !handled {
								return nil, false
							}
							yes, ok := result.Value.(bool)
							if !result.Defined || !ok {
								return nil, false
							}
							if !yes {
								continue
							}
						}
						child, found := item.Member(n.children[1].val.(string))
						if !found {
							continue
						}
						var value any
						switch child.Kind() {
						case "null":
						case "boolean":
							value, _ = child.Boolean()
						case "number":
							x, _ := child.Binary64()
							value = x + 0 // -0 prints as 0, as in the reference
						case "string":
							x, _ := child.Text()
							value = x
						default:
							return nil, false
						}
						if evalErr = bd.Collection(len(out) + 1); evalErr != nil {
							return nil, false
						}
						out = append(out, value)
					}
					if len(out) == 0 {
						return Undefined, true
					}
					if len(out) == 1 {
						return out[0], true
					}
					return out, true
				}
			}
			v, found, ok := root, true, true
			if n.op != "variable" {
				v, found, ok = path(n, root)
			}
			if !ok {
				if n == p.root && p.projection != nil && evalErr == nil {
					r, handled, err := p.evalProjection(root, bd)
					evalErr = err
					if !handled {
						return nil, false
					}
					if !r.Defined {
						return Undefined, true
					}
					return r.Value, true
				}
				return nil, false
			}
			if !found {
				return Undefined, true
			}
			switch v.Kind() {
			case "null":
				return nil, true
			case "boolean":
				x, _ := v.Boolean()
				return x, true
			case "number":
				x, _ := v.Binary64()
				return x + 0, true // -0 prints as 0, as in the reference
			case "string":
				x, _ := v.Text()
				return x, true
			default:
				return nil, false
			}
		case "number", "string", "literal":
			return n.val, true
		case "call":
			arg := n.children[1]
			if (arg.op == "name" || arg.op == "variable" || arg.op == "." || arg.op == "filter") && (n.children[0].val == "$count" || n.children[0].val == "$sum") {
				v, found, ok := path(arg, root)
				if evalErr != nil {
					return nil, false
				}
				if ok && found && v.Kind() == "array" {
					if evalErr = bd.Collection(v.Len()); evalErr != nil {
						return nil, false
					}
					if n.children[0].val == "$count" {
						return float64(v.Len()), true
					}
					var sum float64
					for i := 0; i < v.Len(); i++ {
						if evalErr = bd.Step(); evalErr != nil {
							return nil, false
						}
						item, _ := v.Index(i)
						number, ok := item.Binary64()
						if !ok {
							return nil, false
						}
						sum += number
						if math.IsInf(sum, 0) || math.IsNaN(sum) {
							return nil, false
						}
					}
					return sum, true
				}
			}
			a, ok := eval(n.children[1])
			if !ok {
				return nil, false
			}
			switch n.children[0].val.(string) {
			case "$exists":
				return !IsUndefined(a), true
			case "$count":
				if IsUndefined(a) {
					return float64(0), true
				}
				return float64(1), true
			case "$type":
				if IsUndefined(a) {
					return Undefined, true
				}
				switch a.(type) {
				case nil:
					return "null", true
				case bool:
					return "boolean", true
				case float64:
					return "number", true
				case string:
					return "string", true
				}
			case "$sum":
				if IsUndefined(a) {
					return Undefined, true
				}
				if x, ok := a.(float64); ok {
					var sum float64
					sum += x
					return sum, true
				}
			case "$length":
				if IsUndefined(a) {
					return Undefined, true
				}
				if s, ok := a.(string); ok {
					for i := 0; i < len(s); i++ {
						if i%4096 == 0 {
							if evalErr = bd.Step(); evalErr != nil {
								return nil, false
							}
						}
						if s[i] >= 128 {
							return nil, false
						}
					}
					return float64(len(s)), true
				}
			}
			return nil, false
		}
		a, ok := eval(n.children[0])
		if !ok {
			return nil, false
		}
		truth := func(v Value) bool {
			if IsUndefined(v) || v == nil {
				return false
			}
			switch x := v.(type) {
			case bool:
				return x
			case float64:
				return x != 0 && !math.IsNaN(x)
			case string:
				return x != ""
			}
			return false
		}
		if n.op == "and" && !truth(a) {
			return false, true
		}
		if n.op == "or" && truth(a) {
			return true, true
		}
		b, ok := eval(n.children[1])
		if !ok {
			return nil, false
		}
		switch n.op {
		case "and", "or":
			return truth(b), true
		case "=", "!=":
			if IsUndefined(a) || IsUndefined(b) {
				return false, true
			}
			equal := false
			if x, ok := a.(string); ok {
				if y, ok := b.(string); ok {
					var err error
					equal, err = fastEqualStrings(x, y, bd.Step)
					if err != nil {
						evalErr = err
						return nil, false
					}
				} else {
					equal = reflect.DeepEqual(a, b)
				}
			} else {
				equal = reflect.DeepEqual(a, b)
			}
			if n.op == "!=" {
				equal = !equal
			}
			return equal, true
		case "<", ">", "<=", ">=":
			var cmp int
			switch x := a.(type) {
			case float64:
				y, ok := b.(float64)
				if !ok {
					return nil, false
				}
				if x < y {
					cmp = -1
				} else if x > y {
					cmp = 1
				}
			case string:
				y, ok := b.(string)
				if !ok {
					return nil, false
				}
				var err error
				cmp, err = jsonvalue.CompareUTF16(x, y, bd.Step)
				if err != nil {
					evalErr = err
					return nil, false
				}
			default:
				return nil, false
			}
			switch n.op {
			case "<":
				return cmp < 0, true
			case ">":
				return cmp > 0, true
			case "<=":
				return cmp <= 0, true
			default:
				return cmp >= 0, true
			}
		}
		return nil, false
	}
	v, ok := eval(p.root)
	if !ok {
		return jsontrans.Result{}, false, evalErr
	}
	if IsUndefined(v) {
		return jsontrans.Result{}, true, evalErr
	}
	return jsontrans.Result{Value: v, Defined: true}, true, evalErr
}

// Compare bounded chunks so cancellation is observed during large string comparisons.
func fastEqualStrings(a, b string, check func() error) (bool, error) {
	if len(a) != len(b) {
		return false, nil
	}
	for len(a) > 0 {
		if err := check(); err != nil {
			return false, err
		}
		n := min(len(a), 4096)
		if a[:n] != b[:n] {
			return false, nil
		}
		a, b = a[n:], b[n:]
	}
	return true, nil
}

// compileProjection admits pure name steps and scalar boolean filters, plus
// numeric arithmetic projections when arithmetic is set (inside $sum).
// Arrays used as final values and non-scalar predicates retain the native walk.
func compileProjection(n *node, arithmetic bool) []*node {
	if n.op != "." {
		return nil
	}
	var steps []*node
	var collect func(*node) bool
	collect = func(n *node) bool {
		if n.op == "." {
			return collect(n.children[0]) && collect(n.children[1])
		}
		if arithmetic && fastArithmeticSupported(n) {
			steps = append(steps, n)
			return true
		}
		if n.op == "name" {
			steps = append(steps, n)
			return true
		}
		if n.op == "filter" && n.children[0].op == "name" {
			switch n.children[1].op {
			case "=", "!=", "<", ">", "<=", ">=", "and", "or":
				if fastProjectionPredicate(n.children[1]) {
					steps = append(steps, n)
					return true
				}
			}
		}
		return false
	}
	if !collect(n) {
		return nil
	}
	return steps
}

func (p *fastPlan) evalProjection(root jsontrans.Node, b *budget.Budget) (jsontrans.Result, bool, error) {
	var out []any
	var sum float64
	count := 0
	emit := func(v any) (bool, error) {
		count++
		if err := b.Collection(count); err != nil {
			return false, err
		}
		if p.sumProjection {
			x, ok := v.(float64)
			if !ok {
				return false, nil
			}
			sum += x
			if math.IsInf(sum, 0) || math.IsNaN(sum) {
				return false, nil
			}
			return true, nil
		}
		out = append(out, v)
		return true, nil
	}
	var walk func(jsontrans.Node, int) (bool, error)
	walk = func(focus jsontrans.Node, step int) (bool, error) {
		if err := b.Enter(); err != nil {
			return false, err
		}
		defer b.Leave()
		if focus.Kind() == "array" {
			for i := 0; i < focus.Len(); i++ {
				child, _ := focus.Index(i)
				ok, err := walk(child, step)
				if !ok || err != nil {
					return ok, err
				}
			}
			return true, nil
		}
		n := p.projection[step]
		if n.op != "name" && n.op != "filter" {
			if step+1 != len(p.projection) {
				return false, nil
			}
			v, ok, err := evalFastArithmetic(n, focus, b)
			if err != nil || !ok {
				return false, err
			}
			return emit(v)
		}
		name := n
		if n.op == "filter" {
			name = n.children[0]
		}
		child, found := focus.Member(name.val.(string))
		if !found {
			return true, nil
		}
		var accept func(jsontrans.Node) (bool, error)
		accept = func(child jsontrans.Node) (bool, error) {
			if err := b.Step(); err != nil {
				return false, err
			}
			if n.op == "filter" {
				if child.Kind() == "array" {
					return false, nil
				}
				r, handled, err := (&fastPlan{root: n.children[1]}).eval(child, b)
				if err != nil || !handled {
					return false, err
				}
				yes, ok := r.Value.(bool)
				if !r.Defined || !ok {
					return false, nil
				}
				if !yes {
					return true, nil
				}
			}
			if step+1 < len(p.projection) {
				return walk(child, step+1)
			}
			var v any
			switch child.Kind() {
			case "null":
			case "boolean":
				v, _ = child.Boolean()
			case "number":
				x, _ := child.Binary64()
				v = x + 0 // -0 prints as 0, as in the reference
			case "string":
				v, _ = child.Text()
			default:
				return false, nil
			}
			return emit(v)
		}
		if n.op == "filter" && child.Kind() == "array" {
			for i := 0; i < child.Len(); i++ {
				item, _ := child.Index(i)
				ok, err := accept(item)
				if !ok || err != nil {
					return ok, err
				}
			}
			return true, nil
		}
		return accept(child)
	}
	ok, err := walk(root, 0)
	if err != nil || !ok {
		return jsontrans.Result{}, false, err
	}
	if p.sumProjection {
		if count == 0 {
			return jsontrans.Result{}, true, nil
		}
		return jsontrans.Result{Value: sum, Defined: true}, true, nil
	}
	if len(out) == 0 {
		return jsontrans.Result{}, true, nil
	}
	if len(out) == 1 {
		return jsontrans.Result{Value: out[0], Defined: true}, true, nil
	}
	return jsontrans.Result{Value: out, Defined: true}, true, nil
}

// Arithmetic projection is numeric-only. Missing and nonnumeric operands use
// the native evaluator, which retains its error code and source position.
func fastArithmeticSupported(n *node) bool {
	switch n.op {
	case "block":
		return len(n.children) == 1 && fastArithmeticSupported(n.children[0])
	case "*", "+", "-":
		return len(n.children) == 2 && fastArithmeticOperand(n.children[0]) && fastArithmeticOperand(n.children[1])
	}
	return false
}
func fastArithmeticOperand(n *node) bool {
	return n.op == "number" || n.op == "name" || fastArithmeticSupported(n)
}
func evalFastArithmetic(n *node, focus jsontrans.Node, b *budget.Budget) (float64, bool, error) {
	if err := b.Enter(); err != nil {
		return 0, false, err
	}
	defer b.Leave()
	switch n.op {
	case "block":
		return evalFastArithmetic(n.children[0], focus, b)
	case "number":
		return n.val.(float64), true, nil
	case "name":
		v, found := focus.Member(n.val.(string))
		if !found {
			return 0, false, nil
		}
		x, ok := v.Binary64()
		return x, ok, nil
	}
	a, ok, err := evalFastArithmetic(n.children[0], focus, b)
	if err != nil || !ok {
		return 0, false, err
	}
	c, ok, err := evalFastArithmetic(n.children[1], focus, b)
	if err != nil || !ok {
		return 0, false, err
	}
	var v float64
	switch n.op {
	case "*":
		v = a * c
	case "+":
		v = a + c
	case "-":
		v = a - c
	default:
		return 0, false, nil
	}
	return v, !math.IsInf(v, 0) && !math.IsNaN(v), nil
}

// fastProjectionPredicate admits scalar comparisons and logical operators over
// names and literals.
func fastProjectionPredicate(n *node) bool {
	switch n.op {
	case "name", "number", "string", "literal":
		return true
	case "=", "!=", "<", ">", "<=", ">=", "and", "or":
		return fastProjectionPredicate(n.children[0]) && fastProjectionPredicate(n.children[1])
	}
	return false
}
