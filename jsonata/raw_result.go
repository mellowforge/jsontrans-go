package jsonata

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

// cloneRawResult detaches a language value without imposing the JSON input
// contract. JSON values consume their encoded size; language-only scalars
// (functions, missing and nonfinite numbers) consume one bounded token.
func cloneRawResult(v Value, b *budget.Budget) (Value, error) {
	c := rawResultCopier{b: b, remaining: b.Limits.MaxOutputBytes, active: make(map[rawResultReference]bool)}
	return c.copy(v)
}

type rawResultReference struct {
	kind    reflect.Kind
	pointer uintptr
	length  int
}
type rawResultCopier struct {
	b         *budget.Budget
	remaining int
	active    map[rawResultReference]bool
}

func (c *rawResultCopier) charge(n int) error {
	if n > c.remaining {
		return fmt.Errorf("%w: raw result size", budget.ErrLimit)
	}
	c.remaining -= n
	return nil
}
func (c *rawResultCopier) copy(v Value) (Value, error) {
	languageScalar := IsUndefined(v)
	if _, ok := v.(Callable); ok {
		languageScalar = true
	}
	if n, ok := v.(float64); ok && (math.IsInf(n, 0) || math.IsNaN(n)) {
		languageScalar = true
	}
	if languageScalar {
		if err := c.b.Enter(); err != nil {
			return nil, err
		}
		defer c.b.Leave()
		if err := c.charge(1); err != nil {
			return nil, err
		}
		return v, nil
	}
	if x, ok := v.(jsonvalue.UTF16Object); ok {
		v = map[string]any(x)
	}
	switch x := v.(type) {
	case Sequence:
		return c.array([]any(x), true)
	case []any:
		if x != nil {
			return c.array(x, false)
		}
		v = nil
	case map[string]any:
		if x != nil {
			return c.object(x)
		}
		v = nil
	}
	// Normalize Go numeric representations before measuring the JSONata
	// binary64 result. Validate json.Number first so malformed spellings cannot
	// turn into zero through normalize's trusted-runtime conversion.
	switch x := v.(type) {
	case json.Number:
		for n := len(x); n > 0; n -= 4096 {
			if err := c.b.Step(); err != nil {
				return nil, err
			}
		}
		if _, err := jsonvalue.Clone(x, c.b, math.MaxInt); err != nil {
			return nil, err
		}
		v = normalize(x)
	case float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		v = normalize(x)
	}
	out, size, err := jsonvalue.CloneDetachedSize(v, c.b, c.remaining)
	if err != nil {
		return nil, err
	}
	out = normalize(out)
	if err := c.charge(size); err != nil {
		return nil, err
	}
	return out, nil
}
func (c *rawResultCopier) array(values []any, sequence bool) (Value, error) {
	if err := c.b.Enter(); err != nil {
		return nil, err
	}
	defer c.b.Leave()
	if err := c.b.Collection(len(values)); err != nil {
		return nil, err
	}
	ref := rawResultReference{reflect.Slice, reflect.ValueOf(values).Pointer(), len(values)}
	if c.active[ref] {
		return nil, fmt.Errorf("%w: cycle", jsonvalue.ErrInvalid)
	}
	c.active[ref] = true
	defer delete(c.active, ref)
	if err := c.charge(2 + max(len(values)-1, 0)); err != nil {
		return nil, err
	}
	// Every element needs at least one byte/token. Check before allocating.
	if len(values) > c.remaining {
		return nil, fmt.Errorf("%w: raw result size", budget.ErrLimit)
	}
	out := make([]any, len(values))
	for i, v := range values {
		value, err := c.copy(v)
		if err != nil {
			return nil, err
		}
		out[i] = value
	}
	if sequence {
		return Sequence(out), nil
	}
	return out, nil
}
func (c *rawResultCopier) object(values map[string]any) (Value, error) {
	if err := c.b.Enter(); err != nil {
		return nil, err
	}
	defer c.b.Leave()
	if err := c.b.Collection(len(values)); err != nil {
		return nil, err
	}
	ref := rawResultReference{reflect.Map, uintptr(reflect.ValueOf(values).UnsafePointer()), len(values)}
	if c.active[ref] {
		return nil, fmt.Errorf("%w: cycle", jsonvalue.ErrInvalid)
	}
	c.active[ref] = true
	defer delete(c.active, ref)
	if err := c.charge(2 + max(len(values)-1, 0)); err != nil {
		return nil, err
	}
	// Empty key, colon and one value token need at least four bytes/member.
	if len(values) > c.remaining/4 {
		return nil, fmt.Errorf("%w: raw result size", budget.ErrLimit)
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		key, size, err := jsonvalue.CloneDetachedSize(k, c.b, c.remaining)
		if err != nil {
			return nil, err
		}
		if err := c.charge(size); err != nil {
			return nil, err
		}
		if err := c.charge(1); err != nil {
			return nil, err
		}
		canonical := normalize(key).(string)
		if _, exists := out[canonical]; exists {
			return nil, fmt.Errorf("%w: duplicate canonical key", jsonvalue.ErrInvalid)
		}
		value, err := c.copy(v)
		if err != nil {
			return nil, err
		}
		out[canonical] = value
	}
	return out, nil
}
