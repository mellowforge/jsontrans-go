package jsonata

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

// cloneRawResult detaches a language value under the output size limit. Unlike
// JSON conversion it admits language-only scalars (functions, missing and
// nonfinite numbers), each consuming one bounded token; JSON-shaped parts are
// still validated and consume their encoded size.
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

var errRawResultSize = fmt.Errorf("%w: raw result size", budget.ErrLimit)

func (c *rawResultCopier) charge(n int) error {
	if n > c.remaining {
		return errRawResultSize
	}
	c.remaining -= n
	return nil
}

// isLanguageScalar reports values that JSON cannot represent but raw results
// keep: missing values, functions and non-finite numbers.
func isLanguageScalar(v Value) bool {
	switch x := v.(type) {
	case Callable:
		return true
	case float64:
		return math.IsInf(x, 0) || math.IsNaN(x)
	}
	return IsUndefined(v)
}

func (c *rawResultCopier) copy(v Value) (Value, error) {
	if x, ok := v.(float32); ok {
		v = float64(x)
	}
	if isLanguageScalar(v) {
		if err := c.b.Enter(); err != nil {
			return nil, err
		}
		defer c.b.Leave()
		if err := c.charge(1); err != nil {
			return nil, err
		}
		return v, nil
	}
	// Containers are copied here; scalars are normalized to the JSONata
	// binary64 representation before measuring. json.Number is validated first
	// so malformed spellings cannot turn into zero.
	switch x := v.(type) {
	case Sequence:
		return c.array([]any(x), true)
	case []any:
		if x != nil {
			return c.array(x, false)
		}
		v = nil
	case jsonvalue.UTF16Object:
		if x != nil {
			return c.object(map[string]any(x))
		}
		v = nil
	case map[string]any:
		if x != nil {
			return c.object(x)
		}
		v = nil
	case json.Number:
		for n := len(x); n > 0; n -= 4096 {
			if err := c.b.Step(); err != nil {
				return nil, err
			}
		}
		if _, err := jsonvalue.Clone(x, c.b, math.MaxInt); err != nil {
			return nil, err
		}
		n, err := x.Float64()
		if err != nil {
			return nil, fmt.Errorf("%w: number out of binary64 range", jsonvalue.ErrInvalid)
		}
		v = n
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
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

// open starts copying a container of n members: it checks the collection
// limit and cycles, marks ref active and charges brackets and separators. The
// caller must Enter first and unmark ref when open succeeds.
func (c *rawResultCopier) open(ref rawResultReference, n int) error {
	if err := c.b.Collection(n); err != nil {
		return err
	}
	if c.active[ref] {
		return fmt.Errorf("%w: cycle", jsonvalue.ErrInvalid)
	}
	if err := c.charge(2 + max(n-1, 0)); err != nil {
		return err
	}
	c.active[ref] = true
	return nil
}

func (c *rawResultCopier) array(values []any, sequence bool) (Value, error) {
	if err := c.b.Enter(); err != nil {
		return nil, err
	}
	defer c.b.Leave()
	ref := rawResultReference{reflect.Slice, reflect.ValueOf(values).Pointer(), len(values)}
	if err := c.open(ref, len(values)); err != nil {
		return nil, err
	}
	defer delete(c.active, ref)
	// Every element needs at least one byte/token. Check before allocating.
	if len(values) > c.remaining {
		return nil, errRawResultSize
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
	ref := rawResultReference{reflect.Map, uintptr(reflect.ValueOf(values).UnsafePointer()), len(values)}
	if err := c.open(ref, len(values)); err != nil {
		return nil, err
	}
	defer delete(c.active, ref)
	// Empty key, colon and one value token need at least four bytes/member.
	if len(values) > c.remaining/4 {
		return nil, errRawResultSize
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		key, size, err := jsonvalue.CloneDetachedSize(k, c.b, c.remaining)
		if err != nil {
			return nil, err
		}
		// The key and its colon.
		if err := c.charge(size + 1); err != nil {
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
