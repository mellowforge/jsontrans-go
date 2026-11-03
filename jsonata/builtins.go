// Portions derived from JSONata 2.2.2, Copyright IBM Corp. 2016, 2018. MIT license; see conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go"
)

func builtinError(code, name string) error {
	return &jsontrans.Error{Processor: "jsonata", Phase: jsontrans.PhaseEvaluate, Code: code, Token: name, Cause: fmt.Errorf("%s: %s", name, code)}
}

// jsSliceIndex resolves a relative index as JavaScript's Array.prototype.slice
// does: NaN is 0, fractions truncate, and negatives count from the end.
func jsSliceIndex(index float64, length int) int {
	if math.IsNaN(index) {
		return 0
	}
	index = math.Trunc(index)
	if index < 0 {
		return int(math.Max(float64(length)+index, 0))
	}
	return int(math.Min(index, float64(length)))
}

// jsNumberString formats an integral or non-finite value as JavaScript's
// Number.prototype.toString does: exponent form from 1e21, and the names NaN
// and Infinity.
func jsNumberString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case math.Abs(f) >= 1e21:
		return strconv.FormatFloat(f, 'e', -1, 64)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// jsUint32 converts as JavaScript's ToUint32, which String.prototype.split
// applies to its limit.
func jsUint32(f float64) int {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(f), 1<<32)
	if m < 0 {
		m += 1 << 32
	}
	return int(m)
}

// countLimit converts a positive limit that the reference compares as
// count < limit: fractions round up and Infinity means no limit (-1).
func countLimit(f float64) int {
	if math.IsInf(f, 1) {
		return -1
	}
	return floatToInt(math.Ceil(f))
}
func builtinArg(a []Value, i int) Value {
	if i < len(a) {
		return a[i]
	}
	return Undefined
}
func builtinArray(v Value) []Value {
	switch x := v.(type) {
	case []any:
		return x
	case Sequence:
		return []Value(x)
	default:
		if IsUndefined(v) {
			return nil
		}
		return []Value{v}
	}
}
func builtinBool(c *CallContext, v Value) (bool, error) {
	active := map[uintptr]bool{}
	var visit func(Value) (bool, error)
	visit = func(v Value) (bool, error) {
		if c.Budget != nil {
			if err := c.Budget.Enter(); err != nil {
				return false, err
			}
			defer c.Budget.Leave()
		}
		if err := builtinStep(c); err != nil {
			return false, err
		}
		switch x := v.(type) {
		case nil, undefinedValue:
			return false, nil
		case bool:
			return x, nil
		case float64:
			// As the reference's isNumeric: infinity is D1001, NaN is false.
			if math.IsInf(x, 0) {
				return false, builtinError("D1001", "boolean")
			}
			return x != 0 && !math.IsNaN(x), nil
		case string:
			return len(x) > 0, nil
		case []any:
			if len(x) == 0 {
				return false, nil
			}
			id := reflect.ValueOf(x).Pointer()
			if active[id] {
				return false, fmt.Errorf("%w: cyclic array", jsontrans.ErrInvalidInput)
			}
			active[id] = true
			defer delete(active, id)
			// The reference casts every element of a longer array, so a later
			// element can still raise an error.
			any := false
			for _, y := range x {
				yes, err := visit(y)
				if err != nil {
					return false, err
				}
				any = any || yes
			}
			return any, nil
		case Sequence:
			if len(x) == 0 {
				return false, nil
			}
			id := reflect.ValueOf(x).Pointer()
			if active[id] {
				return false, fmt.Errorf("%w: cyclic array", jsontrans.ErrInvalidInput)
			}
			active[id] = true
			defer delete(active, id)
			// The reference casts every element of a longer array, so a later
			// element can still raise an error.
			any := false
			for _, y := range x {
				yes, err := visit(y)
				if err != nil {
					return false, err
				}
				any = any || yes
			}
			return any, nil
		case map[string]any:
			return len(x) > 0, nil
		}
		return false, nil
	}
	return visit(v)
}
func builtinStep(c *CallContext) error {
	if c.Budget != nil {
		return c.Budget.Step()
	}
	return nil
}
func builtinOutput(c *CallContext, n int) error {
	if err := builtinStep(c); err != nil {
		return err
	}
	if c.Budget != nil && (n < 0 || n > c.Budget.Limits.MaxOutputBytes) {
		return fmt.Errorf("%w: output size", budget.ErrLimit)
	}
	return nil
}
func builtinCollection(c *CallContext, n int) error {
	if c.Budget != nil {
		return c.Budget.Collection(n)
	}
	return nil
}
func builtins() map[string]Callable {
	m := map[string]Callable{}
	add := func(n string, arity int, fn func(*CallContext, []Value) (Value, error)) {
		m[n] = NativeFunction{N: arity, Fn: fn}
	}
	for _, name := range []string{"sum", "count", "max", "min", "average"} {
		name := name
		add(name, 1, func(c *CallContext, a []Value) (Value, error) {
			if len(a) != 1 {
				return nil, builtinError("T0410", name)
			}
			v := builtinArg(a, 0)
			if name == "count" {
				return float64(len(builtinArray(v))), nil
			}
			if IsUndefined(v) {
				return Undefined, nil
			}
			arr := builtinArray(v)
			if len(arr) == 0 && name != "sum" {
				return Undefined, nil
			}
			var n float64
			for i, x := range arr {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
				f, ok := x.(float64)
				if !ok {
					return nil, builtinError("T0412", name)
				}
				if i == 0 || name == "sum" || name == "average" {
					n += f
				} else if name == "max" && f > n || name == "min" && f < n {
					n = f
				}
			}
			if name == "average" {
				n /= float64(len(arr))
			}
			return n, nil
		})
	}
	for _, name := range []string{"boolean", "not", "exists", "type", "string", "number", "floor", "ceil", "round", "abs", "sqrt", "lowercase", "uppercase", "length", "trim", "base64encode", "base64decode", "encodeUrlComponent", "decodeUrlComponent", "encodeUrl", "decodeUrl"} {
		name := name
		arity := 1
		if name == "round" {
			arity = 2
		}
		add(name, arity, func(c *CallContext, a []Value) (Value, error) {
			v := builtinArg(a, 0)
			if len(a) == 0 && name != "exists" && name != "type" {
				v = c.Focus
			}
			if name == "exists" {
				return !IsUndefined(v), nil
			}
			if IsUndefined(v) {
				return Undefined, nil
			}
			switch name {
			case "boolean":
				return builtinBool(c, v)
			case "not":
				yes, err := builtinBool(c, v)
				return !yes, err
			case "type":
				switch v.(type) {
				case nil:
					return "null", nil
				case bool:
					return "boolean", nil
				case float64:
					return "number", nil
				case string:
					return "string", nil
				case []any, Sequence:
					return "array", nil
				case map[string]any:
					return "object", nil
				case Callable:
					return "function", nil
				}
				return Undefined, nil
			case "string":
				if s, ok := v.(string); ok {
					return s, nil
				}
				if _, ok := v.(Callable); ok {
					return "", nil
				}
				if f, ok := v.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
					return nil, builtinError("D3001", "string")
				}
				converted, err := builtinStringValue(c, v)
				if err != nil {
					return nil, err
				}
				pretty, _ := builtinArg(a, 1).(bool)
				if _, err := builtinJSONSize(c, converted, 0, pretty); err != nil {
					return nil, err
				}
				return builtinOrderedJSON(c, v, pretty)

			case "number":
				switch x := v.(type) {
				case float64:
					return x, nil
				case bool:
					if x {
						return float64(1), nil
					}
					return float64(0), nil
				case string:
					if strings.HasPrefix(strings.ToLower(x), "0x") || strings.HasPrefix(strings.ToLower(x), "0b") || strings.HasPrefix(strings.ToLower(x), "0o") {
						i, e := strconv.ParseUint(x, 0, 64)
						if e == nil {
							return float64(i), nil
						}
					}
					f, e := strconv.ParseFloat(strings.TrimSpace(x), 64)
					if e != nil || math.IsInf(f, 0) || math.IsNaN(f) {
						return nil, builtinError("D3030", name)
					}
					return f, nil
				}
				return nil, builtinError("T0410", name)
			case "floor", "ceil", "round", "abs", "sqrt":
				f, ok := v.(float64)
				if !ok {
					return nil, builtinError("T0410", name)
				}
				switch name {
				case "floor":
					return math.Floor(f), nil
				case "ceil":
					return math.Ceil(f), nil
				case "abs":
					return math.Abs(f), nil
				case "sqrt":
					if f < 0 {
						return nil, builtinError("D3060", name)
					}
					return math.Sqrt(f), nil
				default:
					p := float64(0)
					if x, ok := builtinArg(a, 1).(float64); ok && !math.IsNaN(x) {
						p = x
					}
					// The reference shifts by building "<mantissa>e<exponent+p>"; a
					// fractional or exponent-form precision makes that text NaN.
					if p != math.Trunc(p) || math.Abs(p) >= 1e21 {
						return math.NaN(), nil
					}
					return builtinRound(f, floatToInt(p)), nil
				}
			default:
				s, ok := v.(string)
				if !ok {
					return nil, builtinError("T0410", name)
				}
				switch name {
				case "lowercase":
					return builtinUnicodeCase(c, s, false)
				case "uppercase":
					return builtinUnicodeCase(c, s, true)
				case "length":
					units, err := regexInputUnits(c, s)
					if err != nil {
						return nil, err
					}
					return float64(regexCodePointCount(units)), nil
				case "trim":
					return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }), " "), nil
				case "base64encode":
					units, err := regexInputUnits(c, s)
					if err != nil {
						return nil, err
					}
					if err := builtinOutput(c, base64.StdEncoding.EncodedLen(len(units))); err != nil {
						return nil, err
					}
					raw := make([]byte, len(units))
					for i, r := range units {
						if err := builtinStep(c); err != nil {
							return nil, err
						}
						raw[i] = byte(r)
					}
					return base64.StdEncoding.EncodeToString(raw), nil
				case "base64decode":
					var clean strings.Builder
					for i := 0; i < len(s); i++ {
						if err := builtinStep(c); err != nil {
							return nil, err
						}
						x := s[i]
						if x == '=' {
							break
						}
						if x == '-' {
							x = '+'
						} else if x == '_' {
							x = '/'
						}
						if x >= 'A' && x <= 'Z' || x >= 'a' && x <= 'z' || x >= '0' && x <= '9' || x == '+' || x == '/' {
							count := clean.Len() + 1
							decoded := count / 4 * 3
							if count%4 >= 2 {
								decoded += count%4 - 1
							}
							if err := builtinOutput(c, decoded); err != nil {
								return nil, err
							}
							clean.WriteByte(x)
						}
					}
					encoded := clean.String()
					if len(encoded)%4 == 1 {
						encoded = encoded[:len(encoded)-1]
					}
					raw, e := base64.RawStdEncoding.DecodeString(encoded)
					if e != nil {
						return nil, e
					}
					out := boundedStringBuilder{c: c}
					for _, x := range raw {
						out.WriteRune(rune(x))
						if out.err != nil {
							return nil, out.err
						}
					}
					return out.String(), nil
				case "encodeUrlComponent":
					return builtinEncodeURI(c, s, true)
				case "encodeUrl":
					return builtinEncodeURI(c, s, false)
				default:
					return builtinDecodeURI(c, s, name == "decodeUrlComponent")
				}
			}
		})
	}
	add("substring", 3, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "substring")
		}
		start, ok := builtinArg(a, 1).(float64)
		if !ok {
			return nil, builtinError("T0410", "substring")
		}
		units, err := regexInputUnits(c, s)
		if err != nil {
			return nil, err
		}
		offsets := regexCodePointOffsets(units)
		count := len(offsets) - 1
		// Ported from the reference, which slices a code-point array.
		if float64(count)+start < 0 {
			start = 0
		}
		i := jsSliceIndex(start, count)
		end := count
		if n, ok := builtinArg(a, 2).(float64); ok {
			if n <= 0 {
				return "", nil
			}
			endIndex := float64(count) + start + n
			if start >= 0 {
				endIndex = start + n
			}
			end = jsSliceIndex(endIndex, count)
		}
		if end <= i {
			return "", nil
		}
		return regexUnitsString(units[offsets[i]:offsets[end]]), nil
	})
	for _, name := range []string{"substringBefore", "substringAfter", "contains"} {
		name := name
		add(name, 2, func(c *CallContext, a []Value) (Value, error) {
			v := builtinArg(a, 0)
			if IsUndefined(v) {
				return Undefined, nil
			}
			s, ok := v.(string)
			if !ok {
				return nil, builtinError("T0410", name)
			}
			p, ok := builtinArg(a, 1).(string)
			if !ok {
				return nil, builtinError("T0410", name)
			}
			units, err := regexInputUnits(c, s)
			if err != nil {
				return nil, err
			}
			needle, err := regexInputUnits(c, p)
			if err != nil {
				return nil, err
			}
			i, err := regexUnitsIndex(c, units, needle, 0)
			if err != nil {
				return nil, err
			}
			if name == "contains" {
				return i >= 0, nil
			}
			if i < 0 {
				return s, nil
			}
			if name == "substringBefore" {
				return regexUnitsString(units[:i]), nil
			}
			return regexUnitsString(units[i+len(needle):]), nil
		})
	}
	add("power", 2, func(c *CallContext, a []Value) (Value, error) {
		if IsUndefined(builtinArg(a, 0)) {
			return Undefined, nil
		}
		x, xok := builtinArg(a, 0).(float64)
		y, yok := builtinArg(a, 1).(float64)
		if !xok || !yok {
			return nil, builtinError("T0410", "power")
		}
		v := math.Pow(x, y)
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, builtinError("D3061", "power")
		}
		return v, nil
	})
	add("random", 0, func(c *CallContext, _ []Value) (Value, error) {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 53))
		if err != nil {
			return nil, err
		}
		return float64(n.Int64()) / (1 << 53), nil
	})
	add("millis", 0, func(c *CallContext, _ []Value) (Value, error) { return float64(c.Now.UnixMilli()), nil })
	add("append", 2, func(c *CallContext, a []Value) (Value, error) {
		x, y := builtinArg(a, 0), builtinArg(a, 1)
		if IsUndefined(x) {
			return y, nil
		}
		if IsUndefined(y) {
			return x, nil
		}
		left, right := builtinArray(x), builtinArray(y)
		if len(left) > int(^uint(0)>>1)-len(right) {
			return nil, fmt.Errorf("%w: collection size", budget.ErrLimit)
		}
		if err := builtinCollection(c, len(left)+len(right)); err != nil {
			return nil, err
		}
		out := make([]Value, 0, len(left)+len(right))
		out = append(out, left...)
		out = append(out, right...)
		return out, nil
	})
	add("reverse", 1, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		arr := builtinArray(v)
		if err := builtinCollection(c, len(arr)); err != nil {
			return nil, err
		}
		out := make([]Value, len(arr))
		for i, x := range arr {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			out[len(arr)-i-1] = x
		}
		return out, nil
	})
	add("distinct", 1, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		arr := builtinArray(v)
		out := Sequence{}
		for _, x := range arr {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			found := false
			for _, y := range out {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
				equal, err := builtinDeepEqual(c, x, y)
				if err != nil {
					return nil, err
				}
				if equal {
					found = true
					break
				}
			}
			if !found {
				if err := builtinCollection(c, len(out)+1); err != nil {
					return nil, err
				}
				out = append(out, x)
			}
		}
		if _, explicit := v.([]any); explicit {
			return []any(out), nil
		}
		if len(out) == 1 {
			return out[0], nil
		}
		return out, nil
	})
	add("join", 2, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		sep := ""
		if s, ok := builtinArg(a, 1).(string); ok {
			sep = s
		}
		arr := builtinArray(v)
		if err := builtinCollection(c, len(arr)); err != nil {
			return nil, err
		}
		parts := make([]string, len(arr))
		for i, x := range arr {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			s, ok := x.(string)
			if !ok {
				return nil, builtinError("T0412", "join")
			}
			parts[i] = s
		}
		size := 0
		for _, part := range parts {
			if len(part) > int(^uint(0)>>1)-size {
				return nil, fmt.Errorf("%w: output size", budget.ErrLimit)
			}
			size += len(part)
		}
		if len(parts) > 1 {
			if len(sep) > 0 && (len(parts)-1) > (int(^uint(0)>>1)-size)/len(sep) {
				return nil, fmt.Errorf("%w: output size", budget.ErrLimit)
			}
			size += (len(parts) - 1) * len(sep)
		}
		if err := builtinOutput(c, size); err != nil {
			return nil, err
		}
		return strings.Join(parts, sep), nil
	})
	add("keys", 1, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if len(a) == 0 {
			v = c.Focus
		}
		out := Sequence{}
		seen := map[string]bool{}
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
					if !seen[k] {
						if err := builtinCollection(c, len(out)+1); err != nil {
							return err
						}
						seen[k] = true
						out = append(out, k)
					}
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
	})
	add("lookup", 2, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		key, ok := builtinArg(a, 1).(string)
		if !ok {
			return nil, builtinError("T0410", "lookup")
		}
		var walk func(Value) (Value, error)
		walk = func(v Value) (Value, error) {
			if c.Budget != nil {
				if err := c.Budget.Enter(); err != nil {
					return nil, err
				}
				defer c.Budget.Leave()
			}
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			switch x := v.(type) {
			case map[string]any:
				if y, ok := x[key]; ok {
					return y, nil
				}
			case []any:
				out := Sequence{}
				for _, y := range x {
					z, err := walk(y)
					if err != nil {
						return nil, err
					}
					if !IsUndefined(z) {
						if err := builtinCollection(c, len(out)+1); err != nil {
							return nil, err
						}
						out = append(out, z)
					}
				}
				if len(out) == 1 {
					return out[0], nil
				}
				if len(out) > 0 {
					return out, nil
				}
			case Sequence:
				return walk([]any(x))
			}
			return Undefined, nil
		}
		return walk(v)
	})
	add("merge", 1, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		out := map[string]any{}
		for _, x := range builtinArray(v) {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			obj, ok := x.(map[string]any)
			if !ok {
				return nil, builtinError("T0412", "merge")
			}
			for k, y := range obj {
				if err := builtinStep(c); err != nil {
					return nil, err
				}
				if _, exists := out[k]; !exists {
					if err := builtinCollection(c, len(out)+1); err != nil {
						return nil, err
					}
				}
				out[k] = y
			}
		}
		return out, nil
	})
	add("error", 1, func(c *CallContext, a []Value) (Value, error) { return nil, builtinError("D3137", "error") })
	add("assert", 2, func(c *CallContext, a []Value) (Value, error) {
		yes, err := builtinBool(c, builtinArg(a, 0))
		if err != nil {
			return nil, err
		}
		if !yes {
			return nil, builtinError("D3141", "assert")
		}
		return Undefined, nil
	})
	add("eval", 2, func(c *CallContext, a []Value) (Value, error) {
		if IsUndefined(builtinArg(a, 0)) {
			return Undefined, nil
		}
		s, ok := builtinArg(a, 0).(string)
		if !ok {
			return nil, builtinError("T0410", "eval")
		}
		v := c.Focus
		if !IsUndefined(builtinArg(a, 1)) {
			v = a[1]
		}
		if c.Evaluate == nil {
			return nil, builtinError("D3120", "eval")
		}
		out, err := c.Evaluate(s, v)
		if err == nil {
			return out, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, budget.ErrLimit) {
			return nil, err
		}
		code := "D3121"
		var native *jsontrans.Error
		if errors.As(err, &native) && native.Phase == jsontrans.PhaseCompile {
			code = "D3120"
		}
		return nil, &jsontrans.Error{Processor: "jsonata", Phase: jsontrans.PhaseEvaluate, Code: code, Token: "eval", Cause: err}
	})
	add("formatBase", 2, func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		f, ok := v.(float64)
		if !ok {
			return nil, builtinError("T0410", "formatBase")
		}
		base := float64(10)
		if !IsUndefined(builtinArg(a, 1)) {
			var valid bool
			base, valid = a[1].(float64)
			if !valid {
				return nil, builtinError("T0410", "formatBase")
			}
		}
		base = math.RoundToEven(base)
		if math.IsNaN(base) {
			// The reference's range check misses NaN; Number.toString then
			// throws a RangeError without a JSONata code.
			return nil, errors.New("formatBase: radix must be between 2 and 36")
		}
		if base < 2 || base > 36 {
			return nil, builtinError("D3100", "formatBase")
		}
		f = math.RoundToEven(f)
		if math.IsNaN(f) {
			return "NaN", nil
		}
		if math.IsInf(f, 1) {
			return "Infinity", nil
		}
		if math.IsInf(f, -1) {
			return "-Infinity", nil
		}
		if base == 10 && math.Abs(f) >= 1e21 {
			return strconv.FormatFloat(f, 'e', -1, 64), nil
		}
		i, _ := new(big.Float).SetFloat64(f).Int(nil)
		return i.Text(floatToInt(base)), nil
	})
	builtinInteger(m)
	builtinNumberPictures(m)
	builtinDateTime(m)
	builtinStrings(m)
	builtinRegex(m)
	builtinHigherOrder(m)
	builtinObjects(m)
	return m
}

func builtinRound(f float64, precision int) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return f
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	parts := strings.Split(s, "e")
	ex, _ := strconv.Atoi(parts[1])
	// Like JavaScript's Number(text), overflow is ±Inf and underflow is 0.
	shifted, _ := strconv.ParseFloat(parts[0]+"e"+strconv.Itoa(ex+precision), 64)
	if math.IsInf(shifted, 0) {
		// The reference shifts back through Infinity.toString(), which is NaN.
		return math.NaN()
	}
	rounded := math.RoundToEven(shifted)
	s = strconv.FormatFloat(rounded, 'e', -1, 64)
	parts = strings.Split(s, "e")
	ex, _ = strconv.Atoi(parts[1])
	result, _ := strconv.ParseFloat(parts[0]+"e"+strconv.Itoa(ex-precision), 64)
	if result == 0 {
		// The reference returns 0 for -0, since JSON has no negative zero.
		return 0
	}
	return result
}

func builtinStringValue(c *CallContext, v Value) (Value, error) {
	if c.Budget != nil {
		if err := c.Budget.Enter(); err != nil {
			return nil, err
		}
		defer c.Budget.Leave()
	}
	if err := builtinStep(c); err != nil {
		return nil, err
	}
	switch x := v.(type) {
	case undefinedValue:
		return nil, nil
	case Callable:
		return "", nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, builtinError("D1001", "string")
		}
		if math.Trunc(x) != x {
			n, e := strconv.ParseFloat(strconv.FormatFloat(x, 'g', 15, 64), 64)
			if e == nil {
				return n, nil
			}
		}
		return x + 0, nil // -0 prints as 0
	case []any:
		if err := builtinCollection(c, len(x)); err != nil {
			return nil, err
		}
		out := make([]any, len(x))
		for i, y := range x {
			z, e := builtinStringValue(c, y)
			if e != nil {
				return nil, e
			}
			out[i] = z
		}
		return out, nil
	case Sequence:
		return builtinStringValue(c, []any(x))
	case map[string]any:
		if err := builtinCollection(c, len(x)); err != nil {
			return nil, err
		}
		out := make(map[string]any, len(x))
		for k, y := range x {
			if IsUndefined(y) {
				continue
			}
			z, e := builtinStringValue(c, y)
			if e != nil {
				return nil, e
			}
			out[k] = z
		}
		return out, nil
	default:
		return v, nil
	}
}

func builtinJSONSize(c *CallContext, v Value, depth int, pretty bool) (int, error) {
	if err := builtinStep(c); err != nil {
		return 0, err
	}
	n := 0
	add := func(x int) error {
		if x < 0 || n > int(^uint(0)>>1)-x {
			return fmt.Errorf("%w: output size", budget.ErrLimit)
		}
		n += x
		return builtinOutput(c, n)
	}
	var items []Value
	var keys []string
	switch x := v.(type) {
	case []any:
		items = x
	case map[string]any:
		keys = make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		items = make([]Value, len(keys))
		for i, k := range keys {
			items[i] = x[k]
		}
	default:
		if s, ok := v.(string); ok {
			size, err := builtinEncodedStringSize(c, s)
			if err != nil {
				return 0, err
			}
			return size, nil
		}
		raw, e := builtinJSONMarshal(v)
		if e != nil {
			return 0, e
		}
		if err := add(len(raw)); err != nil {
			return 0, err
		}
		return n, nil
	}
	if err := add(2); err != nil {
		return 0, err
	}
	for i, x := range items {
		if i > 0 {
			if err := add(1); err != nil {
				return 0, err
			}
		}
		if pretty {
			if err := add(1 + 2*(depth+1)); err != nil {
				return 0, err
			}
		}
		if keys != nil {
			key := keys[i]
			size, e := builtinEncodedStringSize(c, key)
			if e != nil {
				return 0, e
			}
			if err := add(size + 1); err != nil {
				return 0, err
			}
			if pretty {
				if err := add(1); err != nil {
					return 0, err
				}
			}
		}
		size, e := builtinJSONSize(c, x, depth+1, pretty)
		if e != nil {
			return 0, e
		}
		if err := add(size); err != nil {
			return 0, err
		}
	}
	if pretty && len(items) > 0 {
		if err := add(1 + 2*depth); err != nil {
			return 0, err
		}
	}
	return n, nil
}

func builtinDeepEqual(c *CallContext, a, b Value) (bool, error) {
	if c.Budget != nil {
		if err := c.Budget.Enter(); err != nil {
			return false, err
		}
		defer c.Budget.Leave()
	}
	if err := builtinStep(c); err != nil {
		return false, err
	}
	switch x := a.(type) {
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false, nil
		}
		for i := range x {
			same, err := builtinDeepEqual(c, x[i], y[i])
			if err != nil || !same {
				return same, err
			}
		}
		return true, nil
	case Sequence:
		y, ok := b.(Sequence)
		if !ok || len(x) != len(y) {
			return false, nil
		}
		for i := range x {
			same, err := builtinDeepEqual(c, x[i], y[i])
			if err != nil || !same {
				return same, err
			}
		}
		return true, nil
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false, nil
		}
		for k, v := range x {
			other, ok := y[k]
			if !ok {
				return false, nil
			}
			same, err := builtinDeepEqual(c, v, other)
			if err != nil || !same {
				return same, err
			}
		}
		return true, nil
	default:
		return reflect.DeepEqual(a, b), nil
	}
}

func builtinEncodedStringSize(c *CallContext, s string) (int, error) {
	n := 2
	for len(s) > 0 {
		if err := builtinStep(c); err != nil {
			return 0, err
		}
		r, size := jsonvalue.DecodeRune(s)
		if r == utf8.RuneError && size == 1 {
			return 0, jsontrans.ErrInvalidInput
		}
		s = s[size:]
		switch r {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			n += 2
		default:
			if r < 32 || r >= 0xd800 && r <= 0xdfff {
				n += 6
			} else {
				n += utf8.RuneLen(r)
			}
		}
		if err := builtinOutput(c, n); err != nil {
			return 0, err
		}
	}
	return n, nil
}
