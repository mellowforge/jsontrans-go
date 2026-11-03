package jsonvalue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// UTF16String preserves JSON strings containing isolated UTF-16 surrogate units.
// Its underlying bytes use WTF-8; MarshalJSON writes lossless \uXXXX escapes.
// Ordinary Unicode strings continue to use the built-in string type.
type UTF16String string

func (s UTF16String) MarshalJSON() ([]byte, error) { return Quote(string(s)) }

// DecodeString decodes an already bounded JSON string while preserving UTF-16.
func DecodeString(raw string) (string, error) { return decodeString([]byte(raw)) }

// UTF16Object preserves JSON object keys containing isolated surrogate units.
// Objects with ordinary Unicode keys remain map[string]any.
type UTF16Object map[string]any

func (o UTF16Object) MarshalJSON() ([]byte, error) {
	return marshalContainer(o, make(map[reference]bool), 0)
}
func marshalContainer(v any, active map[reference]bool, depth int) ([]byte, error) {
	if depth > 10000 {
		return nil, fmt.Errorf("%w: JSON depth", ErrInvalid)
	}
	if o, ok := v.(UTF16Object); ok {
		v = map[string]any(o)
	}
	var b bytes.Buffer
	switch x := v.(type) {
	case map[string]any:
		if x == nil {
			return []byte("null"), nil
		}
		ref := reference{reflect.Map, uintptr(reflect.ValueOf(x).UnsafePointer()), len(x)}
		if active[ref] {
			return nil, fmt.Errorf("%w: cycle", ErrInvalid)
		}
		active[ref] = true
		defer delete(active, ref)
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			quoted, err := Quote(key)
			if err != nil {
				return nil, err
			}
			b.Write(quoted)
			b.WriteByte(':')
			value, err := marshalContainer(x[key], active, depth+1)
			if err != nil {
				return nil, err
			}
			b.Write(value)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	case []any:
		if x == nil {
			return []byte("null"), nil
		}
		ref := reference{reflect.Slice, reflect.ValueOf(x).Pointer(), len(x)}
		if active[ref] {
			return nil, fmt.Errorf("%w: cycle", ErrInvalid)
		}
		active[ref] = true
		defer delete(active, ref)
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			value, err := marshalContainer(item, active, depth+1)
			if err != nil {
				return nil, err
			}
			b.Write(value)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	case string:
		return Quote(x)
	case UTF16String:
		return Quote(string(x))
	default:
		return json.Marshal(v)
	}
}

// DecodeRune accepts Unicode scalars and the surrogate extension used by WTF-8.
func DecodeRune(s string) (rune, int) {
	if len(s) >= 3 && s[0] == 0xed && s[1] >= 0xa0 && s[1] <= 0xbf && s[2] >= 0x80 && s[2] <= 0xbf {
		return rune(s[0]&15)<<12 | rune(s[1]&63)<<6 | rune(s[2]&63), 3
	}
	return utf8.DecodeRuneInString(s)
}

// CanonicalString joins adjacent UTF-16 high/low surrogate units into their
// ordinary UTF-8 scalar spelling while preserving isolated units losslessly.
func CanonicalString(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	for len(s) > 0 {
		r, n := DecodeRune(s)
		if r >= 0xd800 && r <= 0xdbff && n < len(s) {
			low, size := DecodeRune(s[n:])
			if low >= 0xdc00 && low <= 0xdfff {
				out = utf8.AppendRune(out, utf16.DecodeRune(r, low))
				s = s[n+size:]
				continue
			}
		}
		out = append(out, s[:n]...)
		s = s[n:]
	}
	return string(out)
}
func validString(s string) bool {
	for len(s) > 0 {
		r, n := DecodeRune(s)
		if r == utf8.RuneError && n == 1 {
			return false
		}
		s = s[n:]
	}
	return true
}
func appendUnit(b []byte, u uint16) []byte {
	if u >= 0xd800 && u <= 0xdfff {
		return append(b, byte(0xe0|u>>12), byte(0x80|(u>>6)&63), byte(0x80|u&63))
	}
	return utf8.AppendRune(b, rune(u))
}

// Quote is encoding/json-compatible and additionally preserves lone surrogates.
func Quote(s string) ([]byte, error) {
	s = CanonicalString(s)
	var b bytes.Buffer
	b.WriteByte('"')
	const hex = "0123456789abcdef"
	escaped := func(r rune) {
		b.WriteString(`\u`)
		b.WriteByte(hex[r>>12&15])
		b.WriteByte(hex[r>>8&15])
		b.WriteByte(hex[r>>4&15])
		b.WriteByte(hex[r&15])
	}
	for len(s) > 0 {
		r, n := DecodeRune(s)
		if r == utf8.RuneError && n == 1 {
			return nil, fmt.Errorf("%w: invalid string encoding", ErrInvalid)
		}
		s = s[n:]
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029 || r >= 0xd800 && r <= 0xdfff {
				escaped(r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.Bytes(), nil
}

func decodeString(raw []byte) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", fmt.Errorf("%w: string", ErrInvalid)
	}
	out := make([]byte, 0, len(raw)-2)
	for i := 1; i < len(raw)-1; i++ {
		if raw[i] != '\\' {
			out = append(out, raw[i])
			continue
		}
		i++
		switch raw[i] {
		case '"', '\\', '/':
			out = append(out, raw[i])
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			if i+4 >= len(raw) {
				return "", fmt.Errorf("%w: Unicode escape", ErrInvalid)
			}
			n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			if err != nil {
				return "", err
			}
			i += 4
			u := uint16(n)
			if u >= 0xd800 && u <= 0xdbff && i+6 < len(raw) && raw[i+1] == '\\' && raw[i+2] == 'u' {
				low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
				if err == nil && low >= 0xdc00 && low <= 0xdfff {
					out = utf8.AppendRune(out, utf16.DecodeRune(rune(u), rune(low)))
					i += 6
					continue
				}
			}
			out = appendUnit(out, u)
		default:
			return "", fmt.Errorf("%w: string escape", ErrInvalid)
		}
	}
	return string(out), nil
}

// stringToken recovers the original string spelling because encoding/json
// replaces unpaired surrogate escapes when decoding tokens.
func stringToken(d *json.Decoder, data []byte) (json.Token, error) {
	before := d.InputOffset()
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if _, ok := token.(string); !ok {
		return token, nil
	}
	raw := data[before:d.InputOffset()]
	start := bytes.IndexByte(raw, '"')
	if start < 0 {
		return nil, fmt.Errorf("%w: string token", ErrInvalid)
	}
	s, err := decodeString(raw[start:])
	if err != nil {
		return nil, err
	}
	return s, nil
}
