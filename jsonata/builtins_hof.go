package jsonata

import (
	"crypto/rand"
	"math/big"
	"sort"
)

func builtinHigherOrder(m map[string]Callable) {
	for _, name := range []string{"map", "filter", "single"} {
		name := name
		m[name] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
			v := builtinArg(a, 0)
			if IsUndefined(v) {
				return Undefined, nil
			}
			arr := builtinArray(v)
			fn, ok := builtinArg(a, 1).(Callable)
			if !ok && !(name == "single" && IsUndefined(builtinArg(a, 1))) {
				return nil, builtinError("T0410", name)
			}
			out := Sequence{}
			for i, x := range arr {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
				result := Value(true)
				if ok {
					args := []Value{x}
					if fn.Arity() > 1 {
						args = append(args, float64(i))
					}
					if fn.Arity() > 2 {
						args = append(args, arr)
					}
					var err error
					result, err = c.Invoke(fn, args)
					if err != nil {
						return nil, err
					}
				}
				if name == "map" {
					if !IsUndefined(result) {
						if err := builtinCollection(c, len(out)+1); err != nil {
							return nil, err
						}
						out = append(out, result)
					}
				} else if yes, err := builtinBool(c, result); err != nil {
					return nil, err
				} else if yes {
					if err := builtinCollection(c, len(out)+1); err != nil {
						return nil, err
					}
					out = append(out, x)
				}
			}
			if name == "single" {
				if len(out) == 0 {
					return nil, builtinError("D3139", name)
				}
				if len(out) > 1 {
					return nil, builtinError("D3138", name)
				}
			}
			if len(out) == 0 {
				return Undefined, nil
			}
			if len(out) == 1 {
				return out[0], nil
			}
			return out, nil
		}}
	}
	m["reduce"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		arr := builtinArray(v)
		fn, ok := builtinArg(a, 1).(Callable)
		if !ok {
			return nil, builtinError("T0410", "reduce")
		}
		if fn.Arity() < 2 {
			return nil, builtinError("D3050", "reduce")
		}
		acc := builtinArg(a, 2)
		start := 0
		if IsUndefined(acc) {
			if len(arr) == 0 {
				return Undefined, nil
			}
			acc = arr[0]
			start = 1
		}
		for i := start; i < len(arr); i++ {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			args := []Value{acc, arr[i]}
			if fn.Arity() > 2 {
				args = append(args, float64(i))
			}
			if fn.Arity() > 3 {
				args = append(args, arr)
			}
			var err error
			acc, err = c.Invoke(fn, args)
			if err != nil {
				return nil, err
			}
		}
		return acc, nil
	}}
	m["zip"] = NativeFunction{N: 0, Fn: func(c *CallContext, a []Value) (Value, error) {
		if len(a) == 0 {
			return []any{}, nil
		}
		n := len(builtinArray(a[0]))
		for _, x := range a[1:] {
			if l := len(builtinArray(x)); l < n {
				n = l
			}
		}
		if err := builtinCollection(c, n); err != nil {
			return nil, err
		}
		if err := builtinCollection(c, len(a)); err != nil {
			return nil, err
		}
		out := make([]any, n)
		for i := 0; i < n; i++ {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			row := make([]any, len(a))
			for j, x := range a {
				row[j] = builtinArray(x)[i]
			}
			out[i] = row
		}
		return out, nil
	}}
}

func builtinObjects(m map[string]Callable) {
	m["clone"] = NativeFunction{N: 1, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if len(a) == 0 {
			v = c.Focus
		}
		if IsUndefined(v) {
			return Undefined, nil
		}
		return builtinStringValue(c, v)
	}}
	m["sort"] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		arr := builtinArray(v)
		if err := builtinCollection(c, len(arr)); err != nil {
			return nil, err
		}
		out := append([]any{}, arr...)
		fn, _ := builtinArg(a, 1).(Callable)
		var failure error
		if fn == nil && len(out) > 0 {
			switch out[0].(type) {
			case string:
				for _, x := range out {
					if _, ok := x.(string); !ok {
						return nil, builtinError("D3070", "sort")
					}
				}
			case float64:
				for _, x := range out {
					if _, ok := x.(float64); !ok {
						return nil, builtinError("D3070", "sort")
					}
				}
			default:
				return nil, builtinError("D3070", "sort")
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if failure != nil {
				return false
			}
			if err := builtinStep(c); err != nil {
				failure = err
				return false
			}
			if fn != nil {
				v, err := c.Invoke(fn, []Value{out[j], out[i]})
				if err != nil {
					failure = err
					return false
				}
				yes, err := builtinBool(c, v)
				if err != nil {
					failure = err
					return false
				}
				return yes
			}
			switch x := out[i].(type) {
			case string:
				return x < out[j].(string)
			case float64:
				return x < out[j].(float64)
			}
			return false
		})
		if failure != nil {
			return nil, failure
		}
		return out, nil
	}}
	m["shuffle"] = NativeFunction{N: 1, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		arr := builtinArray(v)
		if err := builtinCollection(c, len(arr)); err != nil {
			return nil, err
		}
		out := append([]any{}, arr...)
		for i := len(out) - 1; i > 0; i-- {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			j, e := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
			if e != nil {
				return nil, e
			}
			out[i], out[j.Int64()] = out[j.Int64()], out[i]
		}
		return out, nil
	}}

	m["spread"] = NativeFunction{N: 1, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if len(a) == 0 {
			v = c.Focus
		}
		out := Sequence{}
		var visit func(Value) error
		visit = func(v Value) error {
			if c.Budget != nil {
				if err := c.Budget.Enter(); err != nil {
					return err
				}
				defer c.Budget.Leave()
			}
			if err := builtinStep(c); err != nil {
				return err
			}
			switch x := v.(type) {
			case map[string]any:
				if err := builtinCollection(c, len(x)); err != nil {
					return err
				}
				keys, err := builtinObjectKeys(c, x)
				if err != nil {
					return err
				}
				for _, k := range keys {
					if err := builtinCollection(c, len(out)+1); err != nil {
						return err
					}
					out = append(out, map[string]any{k: x[k]})
				}
			case []any:
				for _, y := range x {
					if err := visit(y); err != nil {
						return err
					}
				}
			case Sequence:
				for _, y := range x {
					if err := visit(y); err != nil {
						return err
					}
				}
			default:
				if !IsUndefined(v) {
					out = append(out, v)
				}
			}
			return nil
		}
		if err := visit(v); err != nil {
			return nil, err
		}
		if len(out) == 0 {
			return Undefined, nil
		}
		if len(out) == 1 {
			return out[0], nil
		}
		return out, nil
	}}
	for _, name := range []string{"each", "sift"} {
		name := name
		m[name] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
			v := builtinArg(a, 0)
			if IsUndefined(v) {
				return Undefined, nil
			}
			obj, ok := v.(map[string]any)
			if !ok {
				return nil, builtinError("T0410", name)
			}
			fn, ok := builtinArg(a, 1).(Callable)
			if !ok {
				return nil, builtinError("T0410", name)
			}
			if err := builtinCollection(c, len(obj)); err != nil {
				return nil, err
			}
			keys, err := builtinObjectKeys(c, obj)
			if err != nil {
				return nil, err
			}
			out := Sequence{}
			filtered := map[string]any{}
			for _, k := range keys {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
				args := []Value{obj[k]}
				if fn.Arity() > 1 {
					args = append(args, k)
				}
				if fn.Arity() > 2 {
					args = append(args, obj)
				}
				r, err := c.Invoke(fn, args)
				if err != nil {
					return nil, err
				}
				if name == "each" {
					if !IsUndefined(r) {
						if err := builtinCollection(c, len(out)+1); err != nil {
							return nil, err
						}
						out = append(out, r)
					}
				} else if yes, err := builtinBool(c, r); err != nil {
					return nil, err
				} else if yes {
					if err := builtinCollection(c, len(filtered)+1); err != nil {
						return nil, err
					}
					filtered[k] = obj[k]
				}
			}
			if name == "sift" {
				if len(filtered) == 0 {
					return Undefined, nil
				}
				return filtered, nil
			}
			if len(out) == 0 {
				return Undefined, nil
			}
			if len(out) == 1 {
				return out[0], nil
			}
			return out, nil
		}}
	}
}
