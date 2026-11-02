package jsonata

import (
	"bytes"
	"encoding/json"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"sort"
)

func builtinOrderedJSON(c *CallContext, v Value, pretty bool) (string, error) {
	out := boundedStringBuilder{c: c}
	var write func(Value) error
	write = func(v Value) error {
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
		case undefinedValue:
			out.WriteString("null")
		case Callable:
			out.WriteString(`""`)
		case Sequence:
			return write([]any(x))
		case []any:
			if err := builtinCollection(c, len(x)); err != nil {
				return err
			}
			out.WriteByte('[')
			for i, y := range x {
				if i > 0 {
					out.WriteByte(',')
				}
				if err := write(y); err != nil {
					return err
				}
			}
			out.WriteByte(']')
		case map[string]any:
			if err := builtinCollection(c, len(x)); err != nil {
				return err
			}
			ordered, e := builtinObjectKeys(c, x)
			if e != nil {
				return e
			}
			out.WriteByte('{')
			first := true
			for _, k := range ordered {
				if IsUndefined(x[k]) {
					continue
				}
				if !first {
					out.WriteByte(',')
				}
				first = false
				raw, e := builtinJSONMarshal(k)
				if e != nil {
					return e
				}
				out.WriteString(string(raw))
				out.WriteByte(':')
				if err := write(x[k]); err != nil {
					return err
				}
			}
			out.WriteByte('}')
		default:
			converted, e := builtinStringValue(c, v)
			if e != nil {
				return e
			}
			raw, e := builtinJSONMarshal(converted)
			if e != nil {
				return e
			}
			out.WriteString(string(raw))
		}
		return out.err
	}
	if err := write(v); err != nil {
		return "", err
	}
	if !pretty {
		return out.String(), nil
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, []byte(out.String()), "", "  "); err != nil {
		return "", err
	}
	return formatted.String(), nil
}

func builtinObjectKeys(c *CallContext, x map[string]any) ([]string, error) {
	if err := builtinCollection(c, len(x)); err != nil {
		return nil, err
	}
	var keys []string
	if c.ObjectKeys != nil {
		keys = c.ObjectKeys(x)
	}
	seen := make(map[string]bool, len(x))
	out := make([]string, 0, len(x))
	for _, k := range keys {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		if _, ok := x[k]; ok && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	rest := make([]string, 0, len(x)-len(out))
	for k := range x {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	var failure error
	sort.Slice(rest, func(i, j int) bool {
		if failure != nil {
			return false
		}
		failure = builtinStep(c)
		return rest[i] < rest[j]
	})
	if failure != nil {
		return nil, failure
	}
	return append(out, rest...), nil
}

func builtinJSONMarshal(v Value) ([]byte, error) {
	if value, ok := v.(string); ok {
		data, err := jsonvalue.Quote(value)
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		for i := 0; i < len(data); i++ {
			if data[i] == '\\' && i+5 < len(data) {
				switch string(data[i : i+6]) {
				case `\u2028`:
					out.WriteString("\u2028")
					i += 5
					continue
				case `\u2029`:
					out.WriteString("\u2029")
					i += 5
					continue
				case `\u003c`:
					out.WriteByte('<')
					i += 5
					continue
				case `\u003e`:
					out.WriteByte('>')
					i += 5
					continue
				case `\u0026`:
					out.WriteByte('&')
					i += 5
					continue
				}
			}
			out.WriteByte(data[i])
			if data[i] == '\\' && i+1 < len(data) {
				i++
				out.WriteByte(data[i])
			}
		}
		return out.Bytes(), nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes()[:b.Len()-1], nil
}
