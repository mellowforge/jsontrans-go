// Signature validation follows JSONata's MIT-licensed reference algorithm.
// Copyright IBM Corp. 2016, 2018. See conformance/testdata/jsonata/LICENSE.
package jsonata

import (
	"fmt"
	"regexp"
	"strings"

	jt "github.com/mellowforge/jsontrans-go"
)

type signatureParameter struct {
	typ, pattern, subtype string
	context               bool
	contextPattern        *regexp.Regexp
}
type functionSignature struct {
	definition string
	parameters []signatureParameter
	pattern    *regexp.Regexp
}

func signatureError(code string, index int) error {
	phase, position := jt.PhaseEvaluate, 0
	if strings.HasPrefix(code, "S") {
		phase = jt.PhaseCompile
		position = index
	}
	return &jt.Error{Processor: "jsonata", Phase: phase, Code: code, Position: position, Cause: fmt.Errorf("function signature argument %d", index)}
}
func parseSignature(definition string) (*functionSignature, error) {
	s := &functionSignature{definition: definition}
	if len(definition) < 2 || definition[0] != '<' || definition[len(definition)-1] != '>' {
		return nil, signatureError("S0401", 0)
	}
	for i := 1; i < len(definition)-1; i++ {
		ch := definition[i]
		if ch == ':' {
			break
		}
		param := signatureParameter{typ: string(ch)}
		switch ch {
		case 's', 'n', 'b', 'l', 'o':
			param.pattern = "[" + string(ch) + "m]"
		case 'a', 'x':
			param.pattern = "[asnblfom]"
		case 'f':
			param.pattern = "f"
		case 'j':
			param.pattern = "[asnblom]"
		case '-', '?', '+':
			if len(s.parameters) == 0 {
				return nil, signatureError("S0401", i)
			}
			prev := &s.parameters[len(s.parameters)-1]
			if ch == '-' {
				prev.context = true
				var err error
				prev.contextPattern, err = regexp.Compile("^(?:" + prev.pattern + ")$")
				if err != nil {
					return nil, signatureError("S0401", i)
				}
				prev.pattern += "?"
			} else {
				prev.pattern += string(ch)
			}
			continue
		case '(':
			end := strings.IndexByte(definition[i+1:], ')')
			if end < 0 {
				return nil, signatureError("S0402", i)
			}
			end += i + 1
			choice := definition[i+1 : end]
			if strings.ContainsAny(choice, "<>()") {
				return nil, signatureError("S0402", i)
			}
			for _, t := range choice {
				if !strings.ContainsRune("asnblfojx", t) {
					return nil, signatureError("S0402", i)
				}
			}
			param.typ = "(" + choice + ")"
			param.pattern = "[" + choice + "m]"
			i = end
		case '<':
			if len(s.parameters) == 0 {
				return nil, signatureError("S0401", i)
			}
			prev := &s.parameters[len(s.parameters)-1]
			if prev.typ != "a" && prev.typ != "f" {
				return nil, signatureError("S0401", i)
			}
			end, depth := i+1, 1
			for end < len(definition) && depth > 0 {
				if definition[end] == '<' {
					depth++
				}
				if definition[end] == '>' {
					depth--
				}
				end++
			}
			if depth != 0 {
				return nil, signatureError("S0401", i)
			}
			prev.subtype = definition[i+1 : end-1]
			i = end - 1
			continue
		default:
			return nil, signatureError("S0401", i)
		}
		s.parameters = append(s.parameters, param)
	}
	var expression strings.Builder
	expression.WriteByte('^')
	for _, p := range s.parameters {
		expression.WriteByte('(')
		expression.WriteString(p.pattern)
		expression.WriteByte(')')
	}
	expression.WriteByte('$')
	var err error
	s.pattern, err = regexp.Compile(expression.String())
	if err != nil {
		return nil, signatureError("S0401", 0)
	}
	return s, nil
}
func signatureSymbol(v Value) byte {
	if IsUndefined(v) {
		return 'm'
	}
	switch v.(type) {
	case nil:
		return 'l'
	case string:
		return 's'
	case bool:
		return 'b'
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return 'n'
	case []any, Sequence:
		return 'a'
	case Callable:
		return 'f'
	default:
		return 'o'
	}
}
func signatureArray(v Value) ([]any, bool) {
	switch a := v.(type) {
	case []any:
		return a, true
	case Sequence:
		return []any(a), true
	}
	return nil, false
}
func (s *functionSignature) validate(args []Value, focus Value) ([]Value, error) {
	var symbols strings.Builder
	for _, v := range args {
		symbols.WriteByte(signatureSymbol(v))
	}
	matched := s.pattern.FindStringSubmatch(symbols.String())
	if matched == nil {
		pattern, good := "^", 0
		for _, p := range s.parameters {
			pattern += p.pattern
			re := regexp.MustCompile(pattern)
			m := re.FindString(symbols.String())
			if m == "" && !re.MatchString(symbols.String()) {
				return nil, signatureError("T0410", good+1)
			}
			good = len(m)
		}
		return nil, signatureError("T0410", good+1)
	}
	validated := make([]Value, 0, max(len(args), len(s.parameters)))
	argIndex := 0
	at := func(i int) Value {
		if i < len(args) {
			return args[i]
		}
		return Undefined
	}
	for i, p := range s.parameters {
		capture := matched[i+1]
		if capture == "" {
			if p.context {
				if !p.contextPattern.MatchString(string(signatureSymbol(focus))) {
					return nil, signatureError("T0411", argIndex+1)
				}
				validated = append(validated, focus)
			} else {
				validated = append(validated, at(argIndex))
				argIndex++
			}
			continue
		}
		for _, symbol := range capture {
			arg := at(argIndex)
			if p.typ == "a" && !IsUndefined(arg) {
				array, isArray := signatureArray(arg)
				if p.subtype != "" {
					ok := true
					if isArray {
						for _, item := range array {
							if signatureSymbol(item) != p.subtype[0] {
								ok = false
								break
							}
						}
					} else {
						ok = byte(symbol) == p.subtype[0]
					}
					if !ok {
						return nil, signatureError("T0412", argIndex+1)
					}
				}
				if !isArray {
					arg = []any{arg}
				} else {
					arg = array
				}
			}
			validated = append(validated, arg)
			argIndex++
		}
	}
	return validated, nil
}

type signatureCallable struct {
	name      string
	fn        Callable
	signature *functionSignature
}

func (f *signatureCallable) Arity() int { return f.fn.Arity() }
func (f *signatureCallable) Call(c *CallContext, args []Value) (Value, error) {
	validated, err := f.signature.validate(args, c.Focus)
	if err != nil {
		if e, ok := err.(*jt.Error); ok {
			copy := *e
			copy.Token = f.name
			return nil, &copy
		}
		return nil, err
	}
	return f.fn.Call(c, validated)
}
func wrapSignature(name string, fn Callable, definition string) (Callable, error) {
	sig, err := parseSignature(definition)
	if err != nil {
		return nil, err
	}
	return &signatureCallable{name, fn, sig}, nil
}

var builtinSignatures = map[string]string{
	"sum": "<a<n>:n>", "count": "<a:n>", "max": "<a<n>:n>", "min": "<a<n>:n>", "average": "<a<n>:n>",
	"string": "<x-b?:s>", "substring": "<s-nn?:s>", "substringBefore": "<s-s:s>", "substringAfter": "<s-s:s>",
	"lowercase": "<s-:s>", "uppercase": "<s-:s>", "length": "<s-:n>", "trim": "<s-:s>", "pad": "<s-ns?:s>",
	"match": "<s-f<s:o>n?:a<o>>", "contains": "<s-(sf):b>", "replace": "<s-(sf)(sf)n?:s>", "split": "<s-(sf)n?:a<s>>",
	"join": "<a<s>s?:s>", "formatNumber": "<n-so?:s>", "formatBase": "<n-n?:s>", "formatInteger": "<n-s:s>", "parseInteger": "<s-s:n>",
	"number": "<(nsb)-:n>", "floor": "<n-:n>", "ceil": "<n-:n>", "round": "<n-n?:n>", "abs": "<n-:n>", "sqrt": "<n-:n>", "power": "<n-n:n>", "random": "<:n>",
	"boolean": "<x-:b>", "not": "<x-:b>", "map": "<af>", "zip": "<a+>", "filter": "<af>", "single": "<af?>", "reduce": "<afj?:j>", "sift": "<o-f?:o>",
	"keys": "<x-:a<s>>", "lookup": "<x-s:x>", "append": "<xx:a>", "exists": "<x:b>", "spread": "<x-:a<o>>", "merge": "<a<o>:o>", "reverse": "<a:a>", "each": "<o-f:a>",
	"error": "<s?:x>", "assert": "<bs?:x>", "type": "<x:s>", "sort": "<af?:a>", "shuffle": "<a:a>", "distinct": "<x:x>",
	"base64encode": "<s-:s>", "base64decode": "<s-:s>", "encodeUrlComponent": "<s-:s>", "encodeUrl": "<s-:s>", "decodeUrlComponent": "<s-:s>", "decodeUrl": "<s-:s>",
	"eval": "<sx?:x>", "toMillis": "<s-s?:n>", "fromMillis": "<n-s?s?:s>", "clone": "<(oa)-:o>", "now": "<s?s?:s>", "millis": "<:n>",
}

func withBuiltinSignatures(functions map[string]Callable) map[string]Callable {
	for name, fn := range functions {
		if def, ok := builtinSignatures[name]; ok {
			wrapped, err := wrapSignature(name, fn, def)
			if err == nil {
				functions[name] = wrapped
			}
		}
	}
	return functions
}
