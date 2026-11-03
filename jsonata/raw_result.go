package jsonata

import (
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
	out, size, err := jsonvalue.CloneDetachedSize(v, c.b, c.remaining)
	if err != nil {
		return nil, err
	}
	out = normalize(out)
	// Go numeric inputs still have to fit binary64. Nonfinite language floats
	// were handled above, not introduced through normalization of host values.
	if err := validateFinite(out, c.b); err != nil {
		return nil, err
	}
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
		value, err := c.copy(v)
		if err != nil {
			return nil, err
		}
		out[normalize(key).(string)] = value
	}
	return out, nil
}
