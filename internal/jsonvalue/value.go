// Package jsonvalue validates and detaches JSON-shaped Go values.
package jsonvalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

var ErrInvalid = errors.New("invalid JSON value")
var number = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func Decode(data []byte, b *budget.Budget) (any, error) {
	if len(data) > b.Limits.MaxInputBytes {
		return nil, fmt.Errorf("%w: input size", budget.ErrLimit)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: invalid UTF-8", ErrInvalid)
	}
	if err := b.Check(); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := decodeValue(d, data, b)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON data", ErrInvalid)
	}
	return Clone(v, b, b.Limits.MaxInputBytes)
}

func decodeValue(d *json.Decoder, data []byte, b *budget.Budget) (any, error) {
	if err := b.Enter(); err != nil {
		return nil, err
	}
	defer b.Leave()
	token, err := stringToken(d, data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '[':
		out := []any{}
		for d.More() {
			if err := b.Collection(len(out) + 1); err != nil {
				return nil, err
			}
			v, err := decodeValue(d, data, b)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if _, err = d.Token(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return out, nil
	case '{':
		out := map[string]any{}
		keys := []string{}
		for d.More() {
			if err := b.Step(); err != nil {
				return nil, err
			}
			key, err := stringToken(d, data)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("%w: object key", ErrInvalid)
			}
			if _, exists := out[name]; !exists {
				if err := b.Collection(len(out) + 1); err != nil {
					return nil, err
				}
				keys = append(keys, name)
			}
			v, err := decodeValue(d, data, b)
			if err != nil {
				return nil, err
			}
			out[name] = v
		}
		if _, err = d.Token(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		RememberObject(b.Context(), out, keys)
		return out, nil
	default:
		return nil, fmt.Errorf("%w: unexpected delimiter", ErrInvalid)
	}
}

type reference struct {
	kind    reflect.Kind
	pointer uintptr
	length  int
}

func Clone(v any, b *budget.Budget, maxBytes int) (any, error) {
	c := copier{b: b, remaining: maxBytes, active: make(map[reference]bool)}
	return c.copy(v)
}

// CloneDetached also copies immutable string storage, allowing prepared results
// to outlive their source document without retaining its input buffer.
func CloneDetached(v any, b *budget.Budget, maxBytes int) (any, error) {
	// A finite binary64 JSON scalar needs at most 32 bytes. Avoid formatting
	// it just to check the usual multi-megabyte output budget.
	if n, ok := v.(float64); ok && maxBytes >= 32 && !math.IsNaN(n) && !math.IsInf(n, 0) {
		if err := b.Step(); err != nil {
			return nil, err
		}
		return n, nil
	}
	c := copier{b: b, remaining: maxBytes, active: make(map[reference]bool), detach: true}
	return c.copy(v)
}

// CloneDetachedSize validates and detaches a value while measuring its exact
// encoded JSON size in the same bounded traversal.
func CloneDetachedSize(v any, b *budget.Budget, maxBytes int) (any, int, error) {
	c := copier{b: b, remaining: maxBytes, active: make(map[reference]bool), detach: true}
	out, err := c.copy(v)
	return out, maxBytes - c.remaining, err
}

type copier struct {
	detach    bool
	b         *budget.Budget
	remaining int
	active    map[reference]bool
}

func (c *copier) charge(n int) error {
	c.remaining -= n
	if c.remaining < 0 {
		return fmt.Errorf("%w: JSON size", budget.ErrLimit)
	}
	return nil
}
func (c *copier) copy(v any) (any, error) {
	if err := c.b.Enter(); err != nil {
		return nil, err
	}
	defer c.b.Leave()
	switch x := v.(type) {
	case nil, bool:
		n := 4
		if v == false {
			n = 5
		}
		if err := c.charge(n); err != nil {
			return nil, err
		}
		return x, nil
	case string:
		if !validString(x) {
			return nil, fmt.Errorf("%w: invalid UTF-8", ErrInvalid)
		}
		x = CanonicalString(x)
		if err := c.stringSize(x); err != nil {
			return nil, err
		}
		if c.detach {
			x = strings.Clone(x)
		}
		if !utf8.ValidString(x) {
			return UTF16String(x), nil
		}
		return x, nil
	case UTF16String:
		if !validString(string(x)) {
			return nil, fmt.Errorf("%w: string encoding", ErrInvalid)
		}
		canonical := CanonicalString(string(x))
		if err := c.stringSize(canonical); err != nil {
			return nil, err
		}
		if c.detach {
			canonical = strings.Clone(canonical)
		}
		if utf8.ValidString(canonical) {
			return canonical, nil
		}
		return UTF16String(canonical), nil
	case json.Number:
		if err := c.charge(len(x)); err != nil {
			return nil, err
		}
		if !number.MatchString(string(x)) {
			return nil, fmt.Errorf("%w: number", ErrInvalid)
		}
		if c.detach {
			x = json.Number(strings.Clone(string(x)))
		}
		return x, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("%w: nonfinite number", ErrInvalid)
		}
		encoded, _ := json.Marshal(x)
		if err := c.charge(len(encoded)); err != nil {
			return nil, err
		}
		return x, nil
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return nil, fmt.Errorf("%w: nonfinite number", ErrInvalid)
		}
		encoded, _ := json.Marshal(x)
		if err := c.charge(len(encoded)); err != nil {
			return nil, err
		}
		return x, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if err := c.charge(len(fmt.Sprint(x))); err != nil {
			return nil, err
		}
		return x, nil
	case map[string]any:
		if x == nil {
			if err := c.charge(4); err != nil {
				return nil, err
			}
			return nil, nil
		}
		if err := c.b.Collection(len(x)); err != nil {
			return nil, err
		}
		ref := reference{reflect.Map, uintptr(reflect.ValueOf(x).UnsafePointer()), len(x)}
		if c.active[ref] {
			return nil, fmt.Errorf("%w: cycle", ErrInvalid)
		}
		c.active[ref] = true
		defer delete(c.active, ref)
		if err := c.charge(2 + max(len(x)-1, 0)); err != nil {
			return nil, err
		}
		if len(x) > c.remaining/4 {
			return nil, fmt.Errorf("%w: JSON size", budget.ErrLimit)
		}
		out := make(map[string]any, len(x))
		needsUTF16 := false
		for k, v := range x {
			if !validString(k) {
				return nil, fmt.Errorf("%w: key UTF-8", ErrInvalid)
			}
			k = CanonicalString(k)
			if _, exists := out[k]; exists {
				return nil, fmt.Errorf("%w: duplicate canonical object key", ErrInvalid)
			}
			if !utf8.ValidString(k) {
				needsUTF16 = true
			}
			if err := c.stringSize(k); err != nil {
				return nil, err
			}
			if c.detach {
				k = strings.Clone(k)
			}
			if err := c.charge(1); err != nil {
				return nil, err
			}
			y, err := c.copy(v)
			if err != nil {
				return nil, err
			}
			out[k] = y
		}
		if keys := ObjectKeys(c.b.Context(), x); keys != nil {
			RememberObject(c.b.Context(), out, keys)
		}
		if needsUTF16 {
			return UTF16Object(out), nil
		}
		return out, nil
	case UTF16Object:
		return c.copy(map[string]any(x))
	case []any:
		if x == nil {
			if err := c.charge(4); err != nil {
				return nil, err
			}
			return nil, nil
		}
		if err := c.b.Collection(len(x)); err != nil {
			return nil, err
		}
		ref := reference{reflect.Slice, reflect.ValueOf(x).Pointer(), len(x)}
		if c.active[ref] {
			return nil, fmt.Errorf("%w: cycle", ErrInvalid)
		}
		c.active[ref] = true
		defer delete(c.active, ref)
		if err := c.charge(2 + max(len(x)-1, 0)); err != nil {
			return nil, err
		}
		if len(x) > c.remaining {
			return nil, fmt.Errorf("%w: JSON size", budget.ErrLimit)
		}
		out := make([]any, len(x))
		for i, v := range x {
			y, err := c.copy(v)
			if err != nil {
				return nil, err
			}
			out[i] = y
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrInvalid, v)
	}
}

func Encode(v any, b *budget.Budget) ([]byte, error) {
	v, err := Clone(v, b, b.Limits.MaxOutputBytes)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(data) > b.Limits.MaxOutputBytes {
		return nil, fmt.Errorf("%w: encoded output size", budget.ErrLimit)
	}
	if err := b.Check(); err != nil {
		return nil, err
	}
	return data, nil
}

// stringSize counts encoding/json's escaped representation without allocating it.
func (c *copier) stringSize(s string) error {
	if err := c.charge(2); err != nil {
		return err
	}
	nextCheck := 0
	for i := 0; i < len(s); {
		if i >= nextCheck {
			nextCheck = i + 1024
			if err := c.b.Step(); err != nil {
				return err
			}
		}
		ch := s[i]
		n := 1
		if ch < 0x20 {
			n = 6
			if ch == '\b' || ch == '\f' || ch == '\n' || ch == '\r' || ch == '\t' {
				n = 2
			}
		} else if ch == '"' || ch == '\\' {
			n = 2
		} else if ch == '<' || ch == '>' || ch == '&' {
			n = 6
		} else if ch >= utf8.RuneSelf {
			r, size := DecodeRune(s[i:])
			n = size
			if r == '\u2028' || r == '\u2029' || r >= 0xd800 && r <= 0xdfff {
				n = 6
			}
			if err := c.charge(n); err != nil {
				return err
			}
			i += size
			continue
		}
		if err := c.charge(n); err != nil {
			return err
		}
		i++
	}
	return nil
}
