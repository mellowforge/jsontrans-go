//go:build jsontext_experiment && go1.27 && goexperiment.jsonv2

package jsonvalue

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// Decode selects the experimental materializing decoder. This is deliberately
// opt-in; the library's default behavior and minimum Go version are unchanged.
func Decode(data []byte, b *budget.Budget) (any, error) { return decodeJSONText(data, b) }

func decodeJSONText(data []byte, b *budget.Budget) (any, error) {
	if len(data) > b.Limits.MaxInputBytes {
		return nil, fmt.Errorf("%w: input size", budget.ErrLimit)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: invalid UTF-8", ErrInvalid)
	}
	if err := b.Check(); err != nil {
		return nil, err
	}
	// Invalid UTF-8 bytes remain rejected above. This option admits isolated
	// UTF-16 escapes; decodeString below preserves their code units losslessly.
	d := jsontext.NewDecoder(bytes.NewReader(data), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	v, err := decodeTextValue(d, data, b)
	if err != nil {
		return nil, err
	}
	if _, err = d.ReadToken(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON data", ErrInvalid)
	}
	// Keep the existing validation, ownership and exact size/work accounting.
	return Clone(v, b, b.Limits.MaxInputBytes)
}

func decodeTextValue(d *jsontext.Decoder, data []byte, b *budget.Budget) (any, error) {
	if err := b.Enter(); err != nil {
		return nil, err
	}
	defer b.Leave()
	token, err := readTextToken(d, data)
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
		for kind := d.PeekKind(); kind != ']' && kind != '}'; kind = d.PeekKind() {
			if err := b.Collection(len(out) + 1); err != nil {
				return nil, err
			}
			v, err := decodeTextValue(d, data, b)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if _, err = d.ReadToken(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return out, nil
	case '{':
		out := map[string]any{}
		keys := []string{}
		for kind := d.PeekKind(); kind != ']' && kind != '}'; kind = d.PeekKind() {
			if err := b.Step(); err != nil {
				return nil, err
			}
			key, err := readTextToken(d, data)
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
			v, err := decodeTextValue(d, data, b)
			if err != nil {
				return nil, err
			}
			out[name] = v
		}
		if _, err = d.ReadToken(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		RememberObject(b.Context(), out, keys)
		return out, nil
	default:
		return nil, fmt.Errorf("%w: unexpected delimiter", ErrInvalid)
	}
}

// Never retain a borrowed jsontext token. Decode raw string spans with the
// existing UTF-16 decoder and copy number spelling into an owned Go string.
func readTextToken(d *jsontext.Decoder, data []byte) (any, error) {
	before := d.InputOffset()
	tok, err := d.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case '"':
		raw := data[before:d.InputOffset()]
		start := bytes.IndexByte(raw, '"')
		if start < 0 {
			return nil, fmt.Errorf("%w: string token", ErrInvalid)
		}
		return decodeString(raw[start:])
	case '0':
		return json.Number(tok.String()), nil
	case 'n':
		return nil, nil
	case 't':
		return true, nil
	case 'f':
		return false, nil
	case '[', ']', '{', '}':
		return json.Delim(tok.Kind()), nil
	default:
		return nil, fmt.Errorf("%w: token", ErrInvalid)
	}
}
