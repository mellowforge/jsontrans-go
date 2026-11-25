package jsonata

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dlclark/regexp2/v2"
	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
)

// RegexMatcher is an immutable regex literal. Each call compiles its own
// matcher so timeouts and iteration state are evaluation-local.
type RegexMatcher struct {
	Pattern string
	Flags   string
}

func (RegexMatcher) Arity() int { return 1 }
func (r RegexMatcher) Call(c *CallContext, a []Value) (Value, error) {
	return r.call(c, a, nil)
}

// Each evaluation of a regex literal creates one matcher. Its direct calls and
// .next continuations share an offset, just like RegExp.lastIndex in JSONata.
// The cursor stays race-safe when the callable escapes through EvalRaw.
type regexClosure struct {
	RegexMatcher
	lastIndex atomic.Int64
}

// The reference closure takes a subject and an optional starting offset.
func (*regexClosure) Arity() int { return 2 }

func (r *regexClosure) Call(c *CallContext, args []Value) (Value, error) {
	return r.RegexMatcher.call(c, args, r)
}

type regexContinuation struct{ NativeFunction }

type regexSubject struct {
	units   []rune
	length  Value
	nullish string
}

func (r RegexMatcher) call(c *CallContext, args []Value, state *regexClosure) (Value, error) {
	value := builtinArg(args, 0)
	text, err := regexSubjectString(c, value)
	if err != nil {
		return nil, err
	}
	units, err := regexInputUnits(c, text)
	if err != nil {
		return nil, err
	}
	subject := regexSubject{units: units, length: Undefined}
	switch x := value.(type) {
	case nil, undefinedValue:
		subject.nullish = text
	case string:
		subject.length = float64(len(units))
	case []any:
		subject.length = float64(len(x))
	case Sequence:
		subject.length = float64(len(x))
	case map[string]any:
		if length, ok := x["length"]; ok {
			subject.length = length
		}
	case *regexClosure:
		subject.length = float64(2)
	case explicitFunctionArgument, regexContinuation:
		subject.length = float64(0)
	}
	fromIndex, err := regexToNumber(c, builtinArg(args, 1))
	if err != nil {
		return nil, err
	}
	return r.search(c, subject, max(0, floatToInt(fromIndex)), false, state)
}

func regexSubjectString(c *CallContext, subject Value) (string, error) {
	if text, ok := subject.(string); ok {
		return text, nil
	}
	out := boundedStringBuilder{c: c}
	var appendValue func(Value, int) error
	appendValue = func(value Value, depth int) error {
		if c.Budget != nil {
			if err := c.Budget.Enter(); err != nil {
				return err
			}
			defer c.Budget.Leave()
		} else if depth > 256 {
			return budget.ErrLimit
		}
		switch v := value.(type) {
		case nil:
			out.WriteString("null")
		case undefinedValue:
			out.WriteString("undefined")
		case string:
			out.WriteString(v)
		case bool:
			out.WriteString(strconv.FormatBool(v))
		case float64:
			if v != 0 && math.Abs(v) < 1e-6 {
				out.WriteString(strings.Replace(strconv.FormatFloat(v, 'e', -1, 64), "e-0", "e-", 1))
			} else {
				out.WriteString(jsNumberString(v + 0))
			}
		case []any, Sequence:
			for i, item := range builtinArray(v) {
				if i > 0 {
					if err := out.WriteByte(','); err != nil {
						return err
					}
				}
				if item == nil || IsUndefined(item) {
					continue
				}
				if err := appendValue(item, depth+1); err != nil {
					return err
				}
			}
		case map[string]any:
			_, overridesToString := v["toString"]
			var constructed bool
			if c.state != nil {
				entry := c.state.objectOrder[uintptr(reflect.ValueOf(v).UnsafePointer())]
				constructed = entry.constructed
			}
			if overridesToString || constructed {
				return errors.New("cannot convert object to primitive value")
			}
			out.WriteString("[object Object]")
		case explicitFunctionArgument:
			out.WriteString(regexArgumentFunctionSource)
		case *regexClosure:
			out.WriteString(regexClosureFunctionSource)
		case regexContinuation:
			out.WriteString(regexContinuationFunctionSource)
		case Callable:
			out.WriteString("[object Object]")
		default:
			return builtinError("T0410", "regex")
		}
		return out.err
	}
	if err := appendValue(subject, 0); err != nil {
		return "", err
	}
	return out.String(), nil
}

func regexToNumber(c *CallContext, value Value) (float64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case undefinedValue, explicitFunctionArgument:
		return math.NaN(), nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case float64:
		return v, nil
	}
	text, err := regexSubjectString(c, value)
	if err != nil {
		return 0, err
	}
	// Charge for trimming and parsing the complete subject before either pass.
	for i := 0; i < len(text); i += 64 {
		if err := builtinStep(c); err != nil {
			return 0, err
		}
	}
	text = strings.TrimFunc(text, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
			return true
		}
		return r >= 0x2000 && r <= 0x200a
	})
	switch text {
	case "":
		return 0, nil
	case "Infinity", "+Infinity":
		return math.Inf(1), nil
	case "-Infinity":
		return math.Inf(-1), nil
	}
	if len(text) > 2 && text[0] == '0' {
		base := 0
		switch text[1] {
		case 'x', 'X':
			base = 16
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		}
		if base != 0 {
			var n float64
			for _, digit := range text[2:] {
				d := strings.IndexRune("0123456789abcdef", digit)
				if d < 0 {
					d = strings.IndexRune("0123456789ABCDEF", digit)
				}
				if d < 0 || d >= base {
					return math.NaN(), nil
				}
				n = n*float64(base) + float64(d)
			}
			return n, nil
		}
	}
	if strings.ContainsAny(text, "_xXpPnNiIfF") {
		return math.NaN(), nil
	}
	if n, err := strconv.ParseFloat(text, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		return n, nil
	}
	return math.NaN(), nil
}

// errInvalidRegex marks patterns the reference rejects with an uncoded
// SyntaxError when tokenizing.
var errInvalidRegex = errors.New("invalid regular expression")

// validateLiteral keeps syntax accepted by the pinned Node reference separate
// from regexp2 extensions available to standalone RegexMatcher callers.
func (r RegexMatcher) validateLiteral(c *CallContext) error {
	if strings.Count(r.Flags, "i") > 1 || strings.Count(r.Flags, "m") > 1 {
		return fmt.Errorf("%w: duplicate flag", errInvalidRegex)
	}
	re, err := r.compile(c)
	if err != nil || !strings.Contains(r.Pattern, "(?") {
		return err
	}
	inClass, escaped := false, false
	for i := 0; i < len(r.Pattern); i++ {
		if i%64 == 0 {
			if err := builtinStep(c); err != nil {
				return err
			}
		}
		if escaped {
			escaped = false
			continue
		}
		switch r.Pattern[i] {
		case '\\':
			escaped = true
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if !inClass && i+2 < len(r.Pattern) && r.Pattern[i+1] == '?' {
				switch r.Pattern[i+2] {
				case ':', '<', '=', '!':
					// Noncapturing, named capture, or lookaround.
				default:
					return fmt.Errorf("%w: unsupported group prefix", errInvalidRegex)
				}
			}
		}
	}
	// regexp2 reports group names after decoding Unicode escapes.
	names := make(map[string]struct{})
	for _, name := range re.GetGroupNames() {
		if err := builtinStep(c); err != nil {
			return err
		}
		if name == "" {
			continue
		}
		if _, duplicate := names[name]; duplicate {
			return fmt.Errorf("%w: duplicate capture group name", errInvalidRegex)
		}
		names[name] = struct{}{}
	}
	return nil
}

// compile returns a fresh matcher, so timeouts stay evaluation-local.
func (r RegexMatcher) compile(c *CallContext) (*regexp2.Regexp, error) {
	opts := regexp2.ECMAScript
	if strings.Contains(r.Flags, "i") {
		opts |= regexp2.IgnoreCase
	}
	if strings.Contains(r.Flags, "m") {
		opts |= regexp2.Multiline
	}
	if strings.Contains(r.Flags, "s") {
		opts |= regexp2.Singleline
	}
	if err := builtinStep(c); err != nil {
		return nil, err
	}
	pattern, err := regexPattern(c, r.Pattern)
	if err != nil {
		return nil, err
	}
	re, err := regexp2.Compile(pattern, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRegex, err)
	}
	return re, nil
}
func (r RegexMatcher) search(c *CallContext, subject regexSubject, start int, subsequent bool, state *regexClosure) (Value, error) {
	re, err := r.compile(c)
	if err != nil {
		return nil, err
	}
	timeout := 50 * time.Millisecond
	if c.Budget != nil {
		timeout = c.Budget.Limits.RegexTimeout
		if err := c.Budget.Check(); err != nil {
			return nil, err
		}
		if deadline, ok := c.Budget.Context().Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, c.Budget.Check()
			}
			if remaining < timeout {
				timeout = remaining
			}
		}
	}
	if start > len(subject.units) {
		if state != nil {
			state.lastIndex.Store(0)
		}
		return Undefined, nil
	}
	re.MatchTimeout = timeout
	match, err := re.FindRunesMatchStartingAt(subject.units, start)
	if err != nil {
		if c.Budget != nil {
			if e := c.Budget.Check(); e != nil {
				return nil, e
			}
		}
		if strings.HasPrefix(err.Error(), "match timeout after ") {
			return nil, fmt.Errorf("%w: regex timeout", budget.ErrLimit)
		}
		return nil, err
	}
	if c.Budget != nil {
		if err := c.Budget.Check(); err != nil {
			return nil, err
		}
	}
	if match == nil {
		if state != nil {
			state.lastIndex.Store(0)
		}
		return Undefined, nil
	}
	if state != nil {
		state.lastIndex.Store(int64(match.RuneIndex + match.RuneLength))
	}
	if subsequent && match.RuneLength == 0 {
		return nil, builtinError("D1004", "regex")
	}
	return regexResult(c, r, match, subject, state)
}
func regexResult(c *CallContext, r RegexMatcher, m *regexp2.Match, subject regexSubject, state *regexClosure) (Value, error) {
	if err := builtinStep(c); err != nil {
		return nil, err
	}
	units := subject.units
	groups := m.Groups()
	if err := builtinCollection(c, len(groups)-1); err != nil {
		return nil, err
	}
	g := make([]any, 0, len(groups)-1)
	for _, x := range groups[1:] {
		if len(x.Captures) == 0 {
			g = append(g, Undefined)
		} else {
			g = append(g, regexUnitsString(units[x.RuneIndex:x.RuneIndex+x.RuneLength]))
		}
	}
	start := m.RuneIndex
	end := start + m.RuneLength
	next := regexContinuation{NativeFunction{N: 0, Fn: func(c *CallContext, _ []Value) (Value, error) {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		cursor := end
		if state != nil {
			cursor = int(state.lastIndex.Load())
		}
		if subject.nullish != "" {
			return nil, fmt.Errorf("cannot read properties of %s (reading 'length')", subject.nullish)
		}
		length, err := regexToNumber(c, subject.length)
		if err != nil {
			return nil, err
		}
		if float64(cursor) >= length {
			return Undefined, nil
		}
		if state == nil && m.RuneLength == 0 {
			return nil, builtinError("D1004", "regex")
		}
		return r.search(c, subject, cursor, true, state)
	}}}
	return map[string]any{"match": regexUnitsString(units[start:end]), "start": float64(start), "end": float64(end), "groups": g, "next": next}, nil
}

func builtinRegex(m map[string]Callable) {
	m["replace"] = NativeFunction{N: 4, Fn: builtinReplace}
	oldSplit := m["split"]
	m["split"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		fn, ok := builtinArg(a, 1).(Callable)
		if !ok {
			return oldSplit.Call(c, a)
		}
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "split")
		}
		limit := -1
		if n, ok := builtinArg(a, 2).(float64); ok {
			if n < 0 {
				return nil, builtinError("D3020", "split")
			}
			limit = 0
			if n > 0 {
				limit = countLimit(n)
			}
		}
		if limit == 0 {
			return []any{}, nil
		}
		units, err := regexInputUnits(c, s)
		if err != nil {
			return nil, err
		}
		match, err := c.Invoke(fn, []Value{s})
		if err != nil {
			return nil, err
		}
		out := []any{}
		pos := 0
		for !IsUndefined(match) && (limit < 0 || len(out) < limit) {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			obj, ok := match.(map[string]any)
			if !ok {
				return nil, builtinError("T1010", "split")
			}
			start, sok := obj["start"].(float64)
			end, eok := obj["end"].(float64)
			next, nok := obj["next"].(Callable)
			if !sok || !eok || !nok {
				return nil, builtinError("T1010", "split")
			}
			i, j := floatToInt(start), floatToInt(end)
			if i < pos || j < i || j > len(units) {
				return nil, builtinError("T1010", "split")
			}
			if err := builtinCollection(c, len(out)+1); err != nil {
				return nil, err
			}
			out = append(out, regexUnitsString(units[pos:i]))
			if err := builtinCollection(c, len(out)); err != nil {
				return nil, err
			}
			pos = j
			if limit >= 0 && len(out) >= limit {
				return out, nil
			}
			match, err = c.Invoke(next, nil)
			if err != nil {
				return nil, err
			}
		}
		if err := builtinCollection(c, len(out)+1); err != nil {
			return nil, err
		}
		out = append(out, regexUnitsString(units[pos:]))
		if err := builtinCollection(c, len(out)); err != nil {
			return nil, err
		}
		return out, nil
	}}

	m["match"] = NativeFunction{N: 3, Fn: func(c *CallContext, a []Value) (Value, error) {
		v := builtinArg(a, 0)
		if IsUndefined(v) {
			return Undefined, nil
		}
		s, ok := v.(string)
		if !ok {
			return nil, builtinError("T0410", "match")
		}
		fn, ok := builtinArg(a, 1).(Callable)
		if !ok {
			return nil, builtinError("T0410", "match")
		}
		limit := -1
		if n, ok := builtinArg(a, 2).(float64); ok {
			if n < 0 {
				return nil, builtinError("D3040", "match")
			}
			limit = 0
			if n > 0 {
				limit = countLimit(n)
			}
		}
		if limit == 0 {
			return Undefined, nil
		}
		match, err := c.Invoke(fn, []Value{s})
		if err != nil {
			return nil, err
		}
		out := Sequence{}
		for !IsUndefined(match) && (limit < 0 || len(out) < limit) {
			if err := builtinStep(c); err != nil {
				return nil, err
			}
			if err := builtinCollection(c, len(out)+1); err != nil {
				return nil, err
			}
			obj, ok := match.(map[string]any)
			if !ok {
				return nil, builtinError("T1010", "match")
			}
			out = append(out, map[string]any{"match": obj["match"], "index": obj["start"], "groups": obj["groups"]})
			next, ok := obj["next"].(Callable)
			if !ok {
				return nil, builtinError("T1010", "match")
			}
			if limit >= 0 && len(out) >= limit {
				break
			}
			match, err = c.Invoke(next, nil)
			if err != nil {
				return nil, err
			}
		}
		if len(out) == 0 {
			return Undefined, nil
		}
		if len(out) == 1 {
			return out[0], nil
		}
		return out, nil
	}}
	old := m["contains"]
	m["contains"] = NativeFunction{N: 2, Fn: func(c *CallContext, a []Value) (Value, error) {
		if IsUndefined(builtinArg(a, 1)) {
			return Undefined, nil
		}
		if f, ok := builtinArg(a, 1).(Callable); ok {
			v, err := c.Invoke(f, []Value{builtinArg(a, 0)})
			if err != nil {
				return nil, err
			}
			if yes, e := builtinBool(c, v); e != nil {
				return nil, e
			} else if yes {
				obj, ok := v.(map[string]any)
				if !ok {
					return nil, builtinError("T1010", "contains")
				}
				_, start := obj["start"].(float64)
				_, end := obj["end"].(float64)
				_, groups := obj["groups"].([]any)
				_, next := obj["next"].(Callable)
				if !start && !end && !groups && !next {
					return nil, builtinError("T1010", "contains")
				}
			}
			return !IsUndefined(v), nil
		}
		return old.Call(c, a)
	}}
}

func regexReplacement(c *CallContext, template string, obj map[string]any) (string, error) {
	groups, _ := obj["groups"].([]any)
	maxDigits := 1
	for n := len(groups); n >= 10; n /= 10 {
		maxDigits++
	}
	b := boundedStringBuilder{c: c}
	for i := 0; i < len(template); i++ {
		if template[i] != '$' || i+1 >= len(template) {
			b.WriteByte(template[i])
			continue
		}
		ch := template[i+1]
		if ch == '$' {
			b.WriteByte('$')
			i++
			continue
		}
		if ch >= '0' && ch <= '9' {
			if ch == '0' {
				if match, ok := obj["match"].(string); ok {
					b.WriteString(match)
				}
				i++
				continue
			}
			n := 0
			used := 0
			for j := i + 1; j < len(template) && used < maxDigits && template[j] >= '0' && template[j] <= '9'; j++ {
				candidate := n*10 + int(template[j]-'0')
				if candidate > len(groups) && used > 0 {
					break
				}
				n = candidate
				used++
			}
			i += used
			if n > 0 && n <= len(groups) {
				if s, ok := groups[n-1].(string); ok {
					b.WriteString(s)
				}
			}
			continue
		}
		b.WriteByte('$')
	}
	return b.String(), b.err
}

func builtinReplace(c *CallContext, a []Value) (Value, error) {
	v := builtinArg(a, 0)
	if IsUndefined(v) {
		return Undefined, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, builtinError("T0410", "replace")
	}
	pattern := builtinArg(a, 1)
	replacement := builtinArg(a, 2)
	limit := -1
	if n, ok := builtinArg(a, 3).(float64); ok {
		if n < 0 {
			return nil, builtinError("D3011", "replace")
		}
		limit = 0
		if n > 0 {
			limit = countLimit(n)
		}
	}
	if limit == 0 {
		return s, nil
	}
	units, err := regexInputUnits(c, s)
	if err != nil {
		return nil, err
	}
	var patternUnits []rune
	var match Value
	switch p := pattern.(type) {
	case string:
		if p == "" {
			return nil, builtinError("D3010", "replace")
		}
		patternUnits, err = regexInputUnits(c, p)
		if err != nil {
			return nil, err
		}
		i, e := regexUnitsIndex(c, units, patternUnits, 0)
		if e != nil {
			return nil, e
		}
		if i < 0 {
			return s, nil
		}
		match = map[string]any{"match": p, "start": float64(i), "end": float64(i + len(patternUnits)), "groups": []any{}}
	case Callable:
		match, err = c.Invoke(p, []Value{s})
		if err != nil {
			return nil, err
		}
	default:
		return nil, builtinError("T0410", "replace")
	}
	b := boundedStringBuilder{c: c}
	pos, count := 0, 0
	for !IsUndefined(match) && (limit < 0 || count < limit) {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		obj, ok := match.(map[string]any)
		if !ok {
			return nil, builtinError("T1010", "replace")
		}
		start, _ := obj["start"].(float64)
		end, _ := obj["end"].(float64)
		i, j := floatToInt(start), floatToInt(end)
		if i < pos || j < i || j > len(units) {
			return nil, builtinError("T1010", "replace")
		}
		b.WriteString(regexUnitsString(units[pos:i]))
		switch x := replacement.(type) {
		case string:
			sub, e := regexReplacement(c, x, obj)
			if e != nil {
				return nil, e
			}
			b.WriteString(sub)
		case Callable:
			rv, e := c.Invoke(x, []Value{obj})
			if e != nil {
				return nil, e
			}
			rs, ok := rv.(string)
			if !ok {
				return nil, builtinError("D3012", "replace")
			}
			b.WriteString(rs)
		default:
			return nil, builtinError("T0410", "replace")
		}
		if b.err != nil {
			return nil, b.err
		}
		pos = j
		count++
		if next, ok := obj["next"].(Callable); ok {
			match, err = c.Invoke(next, nil)
			if err != nil {
				return nil, err
			}
		} else if p, ok := pattern.(string); ok {
			i, e := regexUnitsIndex(c, units, patternUnits, pos)
			if e != nil {
				return nil, e
			}
			if i < 0 {
				break
			}
			start := i
			match = map[string]any{"match": p, "start": float64(start), "end": float64(start + len(patternUnits)), "groups": []any{}}
		} else {
			break
		}
	}
	b.WriteString(regexUnitsString(units[pos:]))
	if b.err != nil {
		return nil, b.err
	}
	resultUnits, e := regexInputUnits(c, b.String())
	if e != nil {
		return nil, e
	}
	return regexUnitsString(resultUnits), nil
}

func regexDecodeRune(s string) (rune, int) {
	if len(s) >= 3 && s[0] == 0xed && s[1] >= 0xa0 && s[1] <= 0xbf && s[2] >= 0x80 && s[2] <= 0xbf {
		return rune(s[0]&15)<<12 | rune(s[1]&63)<<6 | rune(s[2]&63), 3
	}
	return utf8.DecodeRuneInString(s)
}
func regexInputUnits(c *CallContext, s string) ([]rune, error) {
	n := 0
	for rest := s; len(rest) > 0; {
		if err := builtinStep(c); err != nil {
			return nil, err
		}
		r, size := regexDecodeRune(rest)
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("%w: invalid string encoding", jsontrans.ErrInvalidInput)
		}
		rest = rest[size:]
		n++
		if r > 0xffff {
			n++
		}
		if err := builtinCollection(c, n); err != nil {
			return nil, err
		}
	}
	out := make([]rune, 0, n)
	for len(s) > 0 {
		r, size := regexDecodeRune(s)
		s = s[size:]
		if r > 0xffff {
			a, b := utf16.EncodeRune(r)
			out = append(out, a, b)
		} else {
			out = append(out, r)
		}
	}
	return out, nil
}
func regexUnitsString(units []rune) string {
	var b strings.Builder
	for i := 0; i < len(units); i++ {
		r := units[i]
		if r >= 0xd800 && r <= 0xdbff && i+1 < len(units) && units[i+1] >= 0xdc00 && units[i+1] <= 0xdfff {
			b.WriteRune(utf16.DecodeRune(r, units[i+1]))
			i++
			continue
		}
		if r >= 0xd800 && r <= 0xdfff {
			b.WriteByte(byte(0xe0 | r>>12))
			b.WriteByte(byte(0x80 | (r>>6)&63))
			b.WriteByte(byte(0x80 | r&63))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
func regexPattern(c *CallContext, s string) (string, error) {
	var b strings.Builder
	for len(s) > 0 {
		if err := builtinStep(c); err != nil {
			return "", err
		}
		r, size := regexDecodeRune(s)
		s = s[size:]
		if r > 0xffff {
			a, z := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04X\u%04X`, a, z)
		} else if r >= 0xd800 && r <= 0xdfff {
			fmt.Fprintf(&b, `\u%04X`, r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), nil
}

type boundedStringBuilder struct {
	c   *CallContext
	b   strings.Builder
	err error
}

func (b *boundedStringBuilder) WriteString(s string) {
	if b.err != nil {
		return
	}
	if b.c.Budget != nil && len(s) > b.c.Budget.Limits.MaxOutputBytes-b.b.Len() {
		b.err = fmt.Errorf("%w: output size", budget.ErrLimit)
		return
	}
	if err := builtinStep(b.c); err != nil {
		b.err = err
		return
	}
	b.b.WriteString(s)
}
func (b *boundedStringBuilder) WriteByte(x byte) error {
	b.WriteString(string([]byte{x}))
	return b.err
}
func (b *boundedStringBuilder) String() string { return b.b.String() }

func (b *boundedStringBuilder) WriteRune(x rune) { b.WriteString(string(x)) }

func regexUnitsIndex(c *CallContext, haystack, needle []rune, start int) (int, error) {
	for i := start; i+len(needle) <= len(haystack); i++ {
		match := true
		for j, r := range needle {
			if err := builtinStep(c); err != nil {
				return -1, err
			}
			if haystack[i+j] != r {
				match = false
				break
			}
		}
		if match {
			return i, nil
		}
	}
	return -1, nil
}

func regexCodePointOffsets(units []rune) []int {
	out := make([]int, 0, len(units)+1)
	for i := 0; i < len(units); i++ {
		out = append(out, i)
		if units[i] >= 0xd800 && units[i] <= 0xdbff && i+1 < len(units) && units[i+1] >= 0xdc00 && units[i+1] <= 0xdfff {
			i++
		}
	}
	return append(out, len(units))
}
func regexCodePointCount(units []rune) int {
	n := 0
	for i := 0; i < len(units); i++ {
		n++
		if units[i] >= 0xd800 && units[i] <= 0xdbff && i+1 < len(units) && units[i+1] >= 0xdc00 && units[i+1] <= 0xdfff {
			i++
		}
	}
	return n
}
