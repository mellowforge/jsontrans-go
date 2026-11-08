package jsonvalue

import (
	"encoding/json"
	"fmt"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// EncodeValidated encodes canonical, detached JSON with an exact encoded size
// established by the prepared-evaluation boundary. It must not be used for
// arbitrary adapter or caller values: those still require validation first.
// Reuse the size proof and visit the owned tree without cloning or normalizing
// it again. Encoding retains its own depth, collection, work and deadline checks.
func EncodeValidated(v any, size int, b *budget.Budget) ([]byte, error) {
	if err := b.Check(); err != nil {
		return nil, err
	}
	if size <= 0 {
		return Encode(v, b)
	}
	if size > b.Limits.MaxOutputBytes {
		return nil, fmt.Errorf("%w: encoded output size", budget.ErrLimit)
	}
	if err := checkEncoding(v, b); err != nil {
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

func checkEncoding(v any, b *budget.Budget) error {
	if err := b.Enter(); err != nil {
		return err
	}
	defer b.Leave()
	switch x := v.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case string:
		return encodingStringWork(x, b)
	case UTF16String:
		return encodingStringWork(string(x), b)
	case json.Number:
		return encodingStringWork(string(x), b)
	case UTF16Object:
		return checkEncoding(map[string]any(x), b)
	case map[string]any:
		if err := b.Collection(len(x)); err != nil {
			return err
		}
		for k, v := range x {
			if err := encodingStringWork(k, b); err != nil {
				return err
			}
			if err := checkEncoding(v, b); err != nil {
				return err
			}
		}
		return nil
	case []any:
		if err := b.Collection(len(x)); err != nil {
			return err
		}
		for _, v := range x {
			if err := checkEncoding(v, b); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported validated type %T", ErrInvalid, v)
	}
}

func encodingStringWork(s string, b *budget.Budget) error {
	// Match the bounded string checkpoints without rescanning validated runes or
	// measuring escaping again. The exact output size is already known.
	for remaining := len(s); remaining > 0; remaining -= min(remaining, 1024) {
		if err := b.Step(); err != nil {
			return err
		}
	}
	return nil
}
