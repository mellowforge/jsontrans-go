package jsonata

import (
	"fmt"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	texttransform "golang.org/x/text/transform"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func builtinStrings(m map[string]Callable) {
	m["split"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "split")
		}
		sep, ok := builtinArg(a, 1).(string)
		if !ok {
			return nil, builtinError("T0410", "split")
		}
		limit := -1
		if x, ok := builtinArg(a, 2).(float64); ok {
			if x < 0 {
				return nil, builtinError("D3020", "split")
			}
			limit = floatToInt(x)
			if limit == 0 {
				return []any{}, nil
			}
		}
		units, err := regexInputUnits(c, s)
		if err != nil {
			return nil, err
		}
		needle, err := regexInputUnits(c, sep)
		if err != nil {
			return nil, err
		}
		out := []any{}
		appendPart := func(part []rune) error {
			if err := builtinCollection(c, len(out)+1); err != nil {
				return err
			}
			out = append(out, regexUnitsString(part))
			return nil
		}
		if len(needle) == 0 {
			for i := range units {
				if limit >= 0 && len(out) >= limit {
					break
				}
				if err := appendPart(units[i : i+1]); err != nil {
					return nil, err
				}
			}
		} else {
			pos := 0
			for {
				idx, e := regexUnitsIndex(c, units, needle, pos)
				if e != nil {
					return nil, e
				}
				if idx < 0 {
					if err := appendPart(units[pos:]); err != nil {
						return nil, err
					}
					break
				}
				if err := appendPart(units[pos:idx]); err != nil {
					return nil, err
				}
				if limit >= 0 && len(out) >= limit {
					break
				}
				pos = idx + len(needle)
			}
		}
		return out, nil
	}}
	m["pad"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "pad")
		}
		width, ok := builtinArg(a, 1).(float64)
		if !ok {
			return nil, builtinError("T0410", "pad")
		}
		ch := " "
		if x, ok := builtinArg(a, 2).(string); ok {
			ch = x
		}
		if ch == "" {
			ch = " "
		}
		source, err := regexInputUnits(c, s)
		if err != nil {
			return nil, err
		}
		pattern, err := regexInputUnits(c, ch)
		if err != nil {
			return nil, err
		}
		offsets := regexCodePointOffsets(pattern)
		pieces := make([]string, len(offsets)-1)
		bytesPerCycle := 0
		for i := range pieces {
			pieces[i] = regexUnitsString(pattern[offsets[i]:offsets[i+1]])
			bytesPerCycle += len(pieces[i])
		}
		existing := regexCodePointCount(source)
		if math.IsNaN(width) {
			return s, nil
		}
		absolute := math.Abs(width)
		if math.IsInf(absolute, 0) || absolute > float64(int(^uint(0)>>1)) {
			return nil, fmt.Errorf("%w: collection size", budget.ErrLimit)
		}
		if c.Budget != nil && absolute > float64(c.Budget.Limits.MaxCollectionItems+existing) {
			return nil, fmt.Errorf("%w: collection size", budget.ErrLimit)
		}
		n := int(absolute) - existing
		if n <= 0 {
			return s, nil
		}
		if err := builtinCollection(c, n); err != nil {
			return nil, err
		}
		if c.Budget != nil {
			remaining := c.Budget.Limits.MaxOutputBytes - len(s)
			if remaining < 0 {
				return nil, fmt.Errorf("%w: output size", budget.ErrLimit)
			}
			cycles := n / len(pieces)
			if cycles > remaining/bytesPerCycle {
				return nil, fmt.Errorf("%w: output size", budget.ErrLimit)
			}
			remaining -= cycles * bytesPerCycle
			for _, piece := range pieces[:n%len(pieces)] {
				remaining -= len(piece)
				if remaining < 0 {
					return nil, fmt.Errorf("%w: output size", budget.ErrLimit)
				}
			}
		}
		b := boundedStringBuilder{c: c}
		if width >= 0 {
			b.WriteString(s)
		}
		for i := 0; i < n; i++ {
			b.WriteString(pieces[i%len(pieces)])
			if b.err != nil {
				return nil, b.err
			}
		}
		if width < 0 {
			b.WriteString(s)
		}
		if b.err != nil {
			return nil, b.err
		}
		return b.String(), nil
	}}
}

func builtinEncodeURI(c *CallContext, s string, component bool) (Value, error) {
	if !utf8.ValidString(s) {
		return nil, builtinError("D3140", "encodeUrl")
	}
	allowed := "-_.!~*'()"
	if !component {
		allowed += ";/?:@&=+$,#"
	}
	const hex = "0123456789ABCDEF"
	b := boundedStringBuilder{c: c}
	for i := 0; i < len(s); i++ {
		x := s[i]
		if x >= 'a' && x <= 'z' || x >= 'A' && x <= 'Z' || x >= '0' && x <= '9' || strings.ContainsRune(allowed, rune(x)) {
			b.WriteByte(x)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[x>>4])
			b.WriteByte(hex[x&15])
		}
	}
	if b.err != nil {
		return nil, b.err
	}
	return b.String(), nil
}
func builtinDecodeURI(c *CallContext, s string, component bool) (Value, error) {
	b := boundedStringBuilder{c: c}
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return nil, builtinError("D3140", "decodeUrl")
		}
		x, e := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if e != nil {
			return nil, builtinError("D3140", "decodeUrl")
		}
		if !component && strings.ContainsRune(";/?:@&=+$,#", rune(x)) {
			b.WriteString(s[i : i+3])
		} else {
			b.WriteByte(byte(x))
		}
		i += 2
	}
	if b.err != nil {
		return nil, b.err
	}
	out := b.String()
	if !utf8.ValidString(out) {
		return nil, builtinError("D3140", "decodeUrl")
	}
	return out, nil
}

func builtinUnicodeCase(c *CallContext, s string, upper bool) (Value, error) {
	var tr texttransform.Transformer
	if upper {
		tr = cases.Upper(language.Und)
	} else {
		tr = cases.Lower(language.Und, cases.HandleFinalSigma(true))
	}
	src := []byte(s)
	out := boundedStringBuilder{c: c}
	buf := make([]byte, 4096)
	for {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		nDst, nSrc, err := tr.Transform(buf, src, true)
		src = src[nSrc:]
		if err == texttransform.ErrShortDst && nDst == 0 && nSrc == 0 {
			capLimit := len(s)
			if c.Budget != nil {
				capLimit = c.Budget.Limits.MaxOutputBytes
			}
			if capLimit <= len(buf) {
				return nil, fmt.Errorf("%w: case mapping buffer", budget.ErrLimit)
			}
			next := len(buf) * 2
			if next > capLimit {
				next = capLimit
			}
			buf = make([]byte, next)
			continue
		}
		if nDst > 0 {
			out.WriteString(string(buf[:nDst]))
			if out.err != nil {
				return nil, out.err
			}
		}
		if err == nil {
			break
		}
		if err != texttransform.ErrShortDst {
			return nil, err
		}
	}
	return out.String(), nil
}
